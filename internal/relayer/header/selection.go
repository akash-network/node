package header

import (
	"errors"
	"fmt"
	"math/big"
	"sort"

	cmttypes "github.com/cometbft/cometbft/types"
)

func Select(source Source, selection Selection) (Source, SelectionReport, error) {
	var report SelectionReport
	if source.Signed == nil || source.Signed.Commit == nil || selection.Trigger < 0 || selection.Limit < 1 {
		return Source{}, report, errors.New("invalid signature selection")
	}

	result := cloneSource(source)
	for _, sig := range result.Signed.Commit.Signatures {
		if sig.BlockIDFlag == cmttypes.BlockIDFlagCommit {
			report.Present++
		}
	}
	report.Kept = report.Present

	// This early return is intentional: maxHeaderSignatures is a trim trigger.
	if report.Present <= selection.Trigger {
		return result, report, nil
	}

	fresh, freshTotal, err := powers(source.Validators)
	if err != nil {
		return Source{}, report, err
	}

	trusted := map[string]*big.Int{}
	trustedTotal := new(big.Int)
	if selection.Trust != nil {
		trusted, trustedTotal, err = powers(source.TrustedNext)
		if err != nil {
			return Source{}, report, err
		}
	}

	type candidate struct {
		index   int
		address string
	}
	candidates := make([]candidate, 0)
	seen := make(map[string]bool)
	for i, sig := range result.Signed.Commit.Signatures {
		address := string(sig.ValidatorAddress)
		if sig.BlockIDFlag != cmttypes.BlockIDFlagCommit || fresh[address] == nil || seen[address] {
			continue
		}
		seen[address] = true
		candidates = append(candidates, candidate{
			index:   i,
			address: address,
		})
	}

	type choice struct {
		kept           map[int]bool
		fresh, trusted *big.Int
	}

	pick := func(margin uint64) choice {
		c := choice{
			kept:    make(map[int]bool),
			fresh:   new(big.Int),
			trusted: new(big.Int),
		}

		add := func(v candidate) {
			c.kept[v.index] = true
			c.fresh.Add(c.fresh, fresh[v.address])
			if p := trusted[v.address]; p != nil {
				c.trusted.Add(c.trusted, p)
			}
		}

		byPower := func(p map[string]*big.Int) []candidate {
			values := make([]candidate, 0)
			for _, v := range candidates {
				if !c.kept[v.index] {
					values = append(values, v)
				}
			}

			value := func(address string) *big.Int {
				if n := p[address]; n != nil {
					return n
				}
				return new(big.Int)
			}

			sort.SliceStable(values, func(i, j int) bool {
				return value(values[i].address).Cmp(value(values[j].address)) > 0
			})

			return values
		}

		if level := selection.Trust; level != nil {
			for _, v := range byPower(trusted) {
				if passes(c.trusted, trustedTotal, level.Numerator, level.Denominator, margin) || trusted[v.address] == nil {
					break
				}
				add(v)
			}
		}

		for _, v := range byPower(fresh) {
			if passes(c.fresh, freshTotal, 2, 3, margin) {
				break
			}
			add(v)
		}

		return c
	}

	strict := func(c choice) bool {
		if !passes(c.fresh, freshTotal, 2, 3, 0) {
			return false
		}
		return selection.Trust == nil || passes(c.trusted, trustedTotal, selection.Trust.Numerator, selection.Trust.Denominator, 0)
	}

	c := pick(100)
	if !strict(c) || len(c.kept) > selection.Limit {
		c = pick(0)
	}

	if !passes(c.fresh, freshTotal, 2, 3, 0) {
		return Source{}, report, &UnsupportedError{
			Reason: fmt.Sprintf("signers hold %s of %s voting power; the verifier needs more than 2/3", c.fresh, freshTotal),
		}
	}
	if !strict(c) {
		return Source{}, report, &UnsupportedError{
			Reason: fmt.Sprintf("signers hold %s of %s trusted voting power; the verifier needs more than %d/%d", c.trusted, trustedTotal, selection.Trust.Numerator, selection.Trust.Denominator),
		}
	}
	if len(c.kept) > selection.Limit {
		return Source{}, report, &UnsupportedError{
			Reason: fmt.Sprintf("a quorum needs %d of %d signatures; at most %d fit one header update", len(c.kept), report.Present, selection.Limit),
		}
	}

	for i := range result.Signed.Commit.Signatures {
		if !c.kept[i] {
			result.Signed.Commit.Signatures[i] = cmttypes.NewCommitSigAbsent()
		}
	}
	report.Kept = len(c.kept)

	return result, report, nil
}

func powers(values []*cmttypes.Validator) (map[string]*big.Int, *big.Int, error) {
	result := make(map[string]*big.Int)
	total := new(big.Int)
	for _, v := range values {
		if v == nil || v.VotingPower < 0 || len(v.Address) != 20 {
			return nil, nil, errors.New("invalid validator power")
		}
		if result[string(v.Address)] != nil {
			return nil, nil, errors.New("duplicate validator address")
		}
		n := big.NewInt(v.VotingPower)
		result[string(v.Address)] = n
		total.Add(total, n)
	}

	return result, total, nil
}

func passes(tallied, all *big.Int, numerator, denominator, margin uint64) bool {
	left := new(big.Int).Mul(tallied, new(big.Int).SetUint64(denominator))
	left.Mul(left, big.NewInt(10000))

	right := new(big.Int).Mul(new(big.Int).SetUint64(numerator), big.NewInt(10000))
	padding := new(big.Int).Mul(new(big.Int).SetUint64(margin), new(big.Int).SetUint64(denominator))
	right.Add(right, padding).Mul(right, all)

	return left.Cmp(right) > 0
}
