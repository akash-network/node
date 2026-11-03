package keeper

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"cosmossdk.io/collections"
	collcodec "cosmossdk.io/collections/codec"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"
)

func TestStrictIDCodec(t *testing.T) {
	id := types.ID{
		Owner:    testOwner(17).String(),
		Height:   9001,
		Sequence: 42,
	}
	parsed, err := types.ParseID(id.String())
	require.NoError(t, err)
	require.Equal(t, id, parsed)
	b := make([]byte, 37)
	n, err := RecordIDKey.Encode(b, id)
	require.NoError(t, err)
	require.Equal(t, 37, n)
	require.Equal(t, "14"+strings.Repeat("11", 20)+"0000000000002329000000000000002a", hex.EncodeToString(b))
	n, parsed, err = RecordIDKey.Decode(b)
	require.NoError(t, err)
	require.Equal(t, id, parsed)
	require.Equal(t, 37, n)

	for length := 0; length < 37; length++ {
		_, _, err := RecordIDKey.Decode(b[:length])
		require.Error(t, err)
		_, _, err = RecordIDKey.DecodeNonTerminal(b[:length])
		require.Error(t, err)
	}

	for _, change := range []func([]byte){
		func(b []byte) { b[0] = 255 },
		func(b []byte) { clear(b[21:29]) }, func(b []byte) { b[21] = 128 }, func(b []byte) { clear(b[29:37]) },
	} {
		bad := bytes.Clone(b)
		change(bad)
		_, _, err := RecordIDKey.Decode(bad)
		require.Error(t, err)
	}

	maximum := types.ID{
		Owner:    testOwner(1).String(),
		Height:   math.MaxInt64,
		Sequence: math.MaxUint64,
	}
	_, err = RecordIDKey.Encode(b, maximum)
	require.NoError(t, err)
	_, parsed, err = RecordIDKey.DecodeNonTerminal(append(b, 1))
	require.NoError(t, err)
	require.Equal(t, maximum, parsed)
	require.Equal(t, 37, RecordIDKey.Size(id))
	require.Equal(t, 37, RecordIDKey.SizeNonTerminal(id))
}

func TestStrictSettlementIDCodec(t *testing.T) {
	id := types.ID{
		Owner:    testOwner(17).String(),
		Height:   9001,
		Sequence: 42,
	}
	for _, fixture := range []struct {
		status types.Status
		hex    string
	}{
		{types.StatusTransferPending, "01"},
		{types.StatusMinted, "02"},
		{types.StatusVoided, "03"},
		{types.StatusRefundPending, "04"},
		{types.StatusRefunded, "05"},
	} {
		key := *types.NewSettlementID(fixture.status, id)
		b := make([]byte, 38)
		n, err := SettlementIDKey.Encode(b, key)
		require.NoError(t, err)
		require.Equal(t, 38, n)
		require.Equal(t, fixture.hex+"14"+strings.Repeat("11", 20)+"0000000000002329000000000000002a", hex.EncodeToString(b))
		n, decoded, err := SettlementIDKey.Decode(b)
		require.NoError(t, err)
		require.Equal(t, key, decoded)
		require.Equal(t, len(b), n)

		for length := 0; length < len(b); length++ {
			_, _, err = SettlementIDKey.Decode(b[:length])
			require.Error(t, err)
			_, _, err = SettlementIDKey.DecodeNonTerminal(b[:length])
			require.Error(t, err)
		}
	}

	owner := "14" + strings.Repeat("11", 20)
	height := "0000000000002329"
	sequence := "000000000000002a"
	for _, malformed := range []string{
		"00" + owner + height + sequence,
		"06" + owner + height + sequence,
		"ff" + owner + height + sequence,
		"01ff" + strings.Repeat("11", 20) + height + sequence,
		"01" + owner + "0000000000000000" + sequence,
		"01" + owner + "8000000000000000" + sequence,
		"01" + owner + height + "0000000000000000",
	} {
		b, err := hex.DecodeString(malformed)
		require.NoError(t, err)
		_, _, err = SettlementIDKey.Decode(b)
		require.Error(t, err, malformed)
		_, _, err = SettlementIDKey.DecodeNonTerminal(b)
		require.Error(t, err, malformed)
	}

	maximum := *types.NewSettlementID(types.StatusRefunded, types.ID{
		Owner:    testOwner(255).String(),
		Height:   math.MaxInt64,
		Sequence: math.MaxUint64,
	})

	b := bytes.Repeat([]byte{0xaa}, 38+1)
	n, err := SettlementIDKey.EncodeNonTerminal(b, maximum)
	require.NoError(t, err)
	require.Equal(t, 38, n)
	require.Equal(t, byte(0xaa), b[n])
	n, decoded, err := SettlementIDKey.DecodeNonTerminal(b)
	require.NoError(t, err)
	require.Equal(t, maximum, decoded)
	require.Equal(t, 38, n)
	require.Equal(t, 38, SettlementIDKey.Size(maximum))
	require.Equal(t, 38, SettlementIDKey.SizeNonTerminal(maximum))
}

func proofPolicy() types.ReporterPolicy {
	return types.ReporterPolicy{
		Version:   1,
		Members:   []string{testOwner(1).String(), testOwner(2).String(), testOwner(3).String(), testOwner(4).String()},
		Threshold: 3,
	}
}

func proofSettlement(status types.Status) types.SettlementData {
	s := types.SettlementData{
		Round:  1,
		Policy: proofPolicy(),
	}

	outcome := types.OutcomeUnspecified
	if status == types.StatusMinted {
		outcome = types.OutcomeMinted
	}

	if status == types.StatusRefundPending || status == types.StatusRefunded {
		outcome = types.OutcomeVoided
	}

	if outcome != types.OutcomeUnspecified {
		for _, member := range s.Policy.Members[:s.Policy.Threshold] {
			s.Votes = append(s.Votes, types.Vote{
				Reporter:      member,
				PolicyVersion: 1,
				Observation: types.Observation{
					Outcome:              outcome,
					Slot:                 7,
					TransactionSignature: bytes.Repeat([]byte{8}, 64),
				},
			})
		}
	}

	if status == types.StatusRefundPending {
		s.ChallengeEnd = 3600
	}

	return s
}

type proofVector struct {
	Name         string `json:"name"`
	Owner        string `json:"owner"`
	OwnerHex     string `json:"owner_hex"`
	Height       string `json:"height"`
	Sequence     string `json:"sequence"`
	Recipient    string `json:"recipient"`
	RecipientHex string `json:"recipient_hex"`
	Amount       string `json:"amount"`
	IDText       string `json:"id_text"`
	TransferKey  string `json:"transfer_key_hex"`
	RecordValue  string `json:"record_value_hex"`
	MintNonce    string `json:"mint_proof_nonce_hex"`
	HeightLE     string `json:"height_le_hex"`
	SequenceLE   string `json:"sequence_le_hex"`
}

func TestProofVectors(t *testing.T) {
	cdc := codec.NewProtoCodec(sdkutil.NewCodecOptions().NewInterfaceRegistry())
	vectors := make([]proofVector, 0, 5)
	for _, v := range []struct {
		name             string
		owner            byte
		height           int64
		sequence, amount uint64
	}{
		{"ordinary", 17, 9001, 42, 123456789}, {"minimum", 1, 1, 1, 1},
		{"amount127", 2, 127, 127, 127}, {"amount128", 3, 128, 128, 128},
		{"maximum", 255, math.MaxInt64, math.MaxUint64, math.MaxUint64},
	} {
		id := types.ID{
			Owner:    testOwner(v.owner).String(),
			Height:   v.height,
			Sequence: v.sequence,
		}

		recipient := types.SolanaAddress{}
		copy(recipient[:], bytes.Repeat([]byte{0x22}, 32))

		if v.name == "minimum" {
			clear(recipient[:])
			recipient[31] = 1
		}

		data := types.RecordData{
			Dest:   recipient.String(),
			Amount: sdk.NewCoin(sdkutil.DenomUakt, sdkmath.NewIntFromUint64(v.amount)),
		}

		require.NoError(t, data.ValidateBasic())
		key, err := TransferKey(id)
		require.NoError(t, err)
		require.Len(t, key, 39)
		value, err := codec.CollValue[types.RecordData](cdc).Encode(data)
		require.NoError(t, err)
		mintNonce := sha256.Sum256(key)
		heightLE := binary.LittleEndian.AppendUint64(nil, uint64(v.height)) //nolint:gosec // Fixture heights are positive int64 values.
		vectors = append(vectors, proofVector{
			Name:         v.name,
			Owner:        id.Owner,
			OwnerHex:     hex.EncodeToString(bytes.Repeat([]byte{v.owner}, 20)),
			Height:       strconv.FormatInt(v.height, 10),
			Sequence:     strconv.FormatUint(v.sequence, 10),
			Recipient:    data.Dest,
			RecipientHex: hex.EncodeToString(recipient[:]),
			Amount:       strconv.FormatUint(v.amount, 10),
			IDText:       id.String(),
			TransferKey:  hex.EncodeToString(key),
			RecordValue:  hex.EncodeToString(value),
			MintNonce:    hex.EncodeToString(mintNonce[:]),
			HeightLE:     hex.EncodeToString(heightLE),
			SequenceLE:   hex.EncodeToString(binary.LittleEndian.AppendUint64(nil, v.sequence)),
		})
	}

	checkVectors(t, "../../../contracts/solana/programs/akt-solana/tests/fixtures/proof_vectors.json", vectors)
}

type settlementVector struct {
	Name       string          `json:"name"`
	Owner      string          `json:"owner"`
	OwnerHex   string          `json:"owner_hex"`
	Height     string          `json:"height"`
	Sequence   string          `json:"sequence"`
	Status     int32           `json:"status"`
	Settlement json.RawMessage `json:"settlement"`
	Policies   json.RawMessage `json:"policies"`
	Key        string          `json:"settlement_key_hex"`
	Value      string          `json:"settlement_value_hex"`
	Nonce      string          `json:"proof_nonce_hex"`
}

func TestSettlementProofVectors(t *testing.T) {
	cdc := codec.NewProtoCodec(sdkutil.NewCodecOptions().NewInterfaceRegistry())
	id := types.ID{
		Owner:    testOwner(17).String(),
		Height:   9001,
		Sequence: 42,
	}

	policies := map[uint64]types.ReporterPolicy{1: proofPolicy()}
	var vectors []settlementVector
	add := func(name string, id types.ID, status types.Status, settlement types.SettlementData, policies map[uint64]types.ReporterPolicy) {
		t.Helper()
		require.NoError(t, settlement.Validate(id, status, policies))
		key, err := SettlementKey(*types.NewSettlementID(status, id))
		require.NoError(t, err)
		require.Len(t, key, 40)
		value, err := codec.CollValue[types.SettlementData](cdc).Encode(settlement)
		require.NoError(t, err)

		if name == "maximum_evidence" {
			require.Len(t, value, 6500)
		}

		js, err := cdc.MarshalJSON(&settlement)
		require.NoError(t, err)
		pj, err := json.Marshal(policies)
		require.NoError(t, err)
		owner, err := sdk.AccAddressFromBech32(id.Owner)
		require.NoError(t, err)
		nonce := sha256.Sum256(key)
		vectors = append(vectors, settlementVector{
			Name:       name,
			Owner:      id.Owner,
			OwnerHex:   hex.EncodeToString(owner),
			Height:     strconv.FormatInt(id.Height, 10),
			Sequence:   strconv.FormatUint(id.Sequence, 10),
			Status:     int32(status),
			Settlement: js,
			Policies:   pj,
			Key:        hex.EncodeToString(key),
			Value:      hex.EncodeToString(value),
			Nonce:      hex.EncodeToString(nonce[:]),
		})
	}

	for _, status := range []types.Status{types.StatusTransferPending, types.StatusMinted, types.StatusVoided, types.StatusRefundPending, types.StatusRefunded} {
		s := proofSettlement(status)
		if status != types.StatusTransferPending {
			s.VoidAuthorization = &types.VoidAuthorization{
				RequestedHeight: 9001,
				RequestedRound:  1,
			}
		}

		add(status.String(), id, status, s, policies)
	}

	members := make([]string, 32)
	for i := range members {
		members[i] = testOwner(byte(i + 1)).String()
	}

	primary := types.ReporterPolicy{
		Version:   math.MaxUint64 - 1,
		Members:   members,
		Threshold: 32,
	}

	witness := types.ReporterPolicy{
		Version:   math.MaxUint64,
		Members:   []string{testOwner(33).String(), testOwner(34).String()},
		Threshold: 2,
	}

	s := types.SettlementData{
		Round:             math.MaxUint64,
		Hold:              types.HoldConflict,
		Policy:            primary,
		ChallengeEnd:      253402300799,
		VetoBy:            testOwner(34).String(),
		VetoReason:        strings.Repeat("v", 256),
		VetoPolicyVersion: witness.Version,
		VoidAuthorization: &types.VoidAuthorization{
			RequestedHeight: math.MaxInt64,
			RequestedRound:  math.MaxUint64,
		},
	}

	for _, member := range members {
		s.Votes = append(s.Votes, types.Vote{
			Reporter:      member,
			PolicyVersion: primary.Version,
			Observation: types.Observation{
				Outcome:              types.OutcomeVoided,
				Slot:                 math.MaxUint64,
				TransactionSignature: bytes.Repeat([]byte{0xff}, 64),
			},
		})
	}

	s.Votes = append(s.Votes, types.Vote{
		Reporter:      testOwner(33).String(),
		PolicyVersion: witness.Version,
		Observation: types.Observation{
			Outcome:              types.OutcomeMinted,
			Slot:                 math.MaxUint64,
			TransactionSignature: bytes.Repeat([]byte{0xff}, 64),
		},
	})

	add("maximum_evidence", types.ID{
		Owner:    testOwner(255).String(),
		Height:   math.MaxInt64,
		Sequence: math.MaxUint64,
	}, types.StatusRefundPending, s, map[uint64]types.ReporterPolicy{primary.Version: primary, witness.Version: witness})

	checkVectors(t, "../../../contracts/solana/programs/akt-solana/tests/fixtures/settlement_vectors.json", vectors)
}

func checkVectors(t *testing.T, path string, vectors any) {
	t.Helper()
	encoded, err := json.MarshalIndent(vectors, "", "  ")
	require.NoError(t, err)
	encoded = append(encoded, '\n')
	if os.Getenv("UPDATE_PROOF_VECTORS") == "1" {
		require.NoError(t, os.WriteFile(path, encoded, 0600))
	}

	want, err := os.ReadFile(path) //nolint:gosec // Only the two repository fixture paths above reach this test helper.
	require.NoError(t, err)
	require.Equal(t, string(want), string(encoded))
}

func TestProofStorageAndChronologicalCodecs(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	record := g.Records[7]
	key, err := TransferKey(record.ID)
	require.NoError(t, err)
	value, err := record.Data.Marshal()
	require.NoError(t, err)
	require.Equal(t, value, ctx.KVStore(k.skey).Get(key))
	key, err = SettlementKey(*types.NewSettlementID(record.Status, record.ID))
	require.NoError(t, err)
	value, err = record.Settlement.Marshal()
	require.NoError(t, err)
	require.Equal(t, value, ctx.KVStore(k.skey).Get(key))

	for _, codec := range []collcodec.KeyCodec[types.ID]{RecordIDKey, chronoIDKey} {
		for _, id := range []types.ID{
			{
				Owner:    testOwner(1).String(),
				Height:   1,
				Sequence: 1,
			},
			{
				Owner:    testOwner(255).String(),
				Height:   math.MaxInt64,
				Sequence: math.MaxUint64,
			},
		} {
			require.Equal(t, 37, codec.Size(id))
			require.Equal(t, codec.Size(id), codec.SizeNonTerminal(id))
			b := bytes.Repeat([]byte{0xaa}, codec.Size(id)+2)
			n, err := codec.EncodeNonTerminal(b, id)
			require.NoError(t, err)
			require.Equal(t, codec.Size(id), n)
			require.Equal(t, []byte{0xaa, 0xaa}, b[n:])
			consumed, got, err := codec.DecodeNonTerminal(b)
			require.NoError(t, err)
			require.Equal(t, n, consumed)
			require.Equal(t, id, got)

			for length := 0; length < n; length++ {
				_, _, err = codec.Decode(b[:length])
				require.Error(t, err)
				_, _, err = codec.DecodeNonTerminal(b[:length])
				require.Error(t, err)
			}
		}
	}

	b := make([]byte, chronoIDKey.Size(record.ID))
	_, err = chronoIDKey.Encode(b, record.ID)
	require.NoError(t, err)
	require.Equal(t, "0000000000000007000000000000000814"+strings.Repeat("07", 20), hex.EncodeToString(b))

	for _, change := range []func([]byte){
		func(b []byte) { clear(b[:8]) },
		func(b []byte) { b[0] = 128 },
		func(b []byte) { clear(b[8:16]) },
		func(b []byte) { b[16] = 255 },
	} {
		bad := bytes.Clone(b)
		change(bad)
		_, _, err = chronoIDKey.Decode(bad)
		require.Error(t, err)
	}
}

func TestIDPrefixRanges(t *testing.T) {
	id := types.ID{
		Owner:    testOwner(17).String(),
		Height:   9001,
		Sequence: 42,
	}

	full := make([]byte, RecordIDKey.Size(id))
	_, err := RecordIDKey.Encode(full, id)
	require.NoError(t, err)
	owner, err := RecordIDKey.ToPrefix(types.ID{Owner: id.Owner})
	require.NoError(t, err)
	require.Equal(t, "14"+strings.Repeat("11", 20), hex.EncodeToString(owner))

	for depth, partial := range []types.ID{
		{},
		{Owner: id.Owner},
		{
			Owner:  id.Owner,
			Height: id.Height,
		},
		id,
	} {
		prefix, err := RecordIDKey.ToPrefix(partial)
		require.NoError(t, err)
		require.Equal(t, []int{0, 21, 29, 37}[depth], len(prefix))
		require.True(t, bytes.HasPrefix(full, prefix))

		for _, other := range []types.ID{
			id,
			{
				Owner:    testOwner(18).String(),
				Height:   id.Height,
				Sequence: id.Sequence,
			},
			{
				Owner:    id.Owner,
				Height:   id.Height + 1,
				Sequence: id.Sequence,
			},
			{
				Owner:    id.Owner,
				Height:   id.Height,
				Sequence: id.Sequence + 1,
			},
		} {
			encoded := make([]byte, RecordIDKey.Size(other))
			_, err = RecordIDKey.Encode(encoded, other)
			require.NoError(t, err)
			matches := (depth < 1 || other.Owner == id.Owner) && (depth < 2 || other.Height == id.Height) && (depth < 3 || other.Sequence == id.Sequence)
			require.Equal(t, matches, bytes.HasPrefix(encoded, prefix))
			end := storetypes.PrefixEndBytes(prefix)
			require.Equal(t, matches, bytes.Compare(encoded, prefix) >= 0 && (end == nil || bytes.Compare(encoded, end) < 0))

			for status := types.StatusTransferPending; status <= types.StatusRefunded; status++ {
				filter := *types.NewSettlementID(types.StatusMinted, partial)
				prefix, err := SettlementIDKey.ToPrefix(filter)
				require.NoError(t, err)
				require.Equal(t, append([]byte{2}, full[:len(prefix)-1]...), prefix)
				key := *types.NewSettlementID(status, other)
				encoded := make([]byte, SettlementIDKey.Size(key))
				_, err = SettlementIDKey.Encode(encoded, key)
				require.NoError(t, err)
				require.Equal(t, matches && status == types.StatusMinted, bytes.HasPrefix(encoded, prefix))
				end := storetypes.PrefixEndBytes(prefix)
				require.Equal(t, matches && status == types.StatusMinted, bytes.Compare(encoded, prefix) >= 0 && bytes.Compare(encoded, end) < 0)
			}
		}
	}

	prefix, err := SettlementIDKey.ToPrefix(types.SettlementID{})
	require.NoError(t, err)
	require.Empty(t, prefix)
	_, err = SettlementIDKey.ToPrefix(types.SettlementID{Status: 256})
	require.Error(t, err)
	_, err = RecordIDKey.ToPrefix(types.ID{Owner: "invalid"})
	require.Error(t, err)

	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)

	for status := types.StatusTransferPending; status <= types.StatusRefunded; status++ {
		prefix, err := SettlementIDKey.ToPrefix(types.SettlementID{Status: status})
		require.NoError(t, err)
		it, err := k.settlements.IterateRaw(ctx, prefix, storetypes.PrefixEndBytes(prefix), collections.OrderAscending)
		require.NoError(t, err)
		keys, err := it.Keys()
		require.NoError(t, err)
		require.NoError(t, it.Close())
		count := 0
		for _, record := range g.Records {
			if record.Status == status {
				count++
			}
		}

		require.Len(t, keys, count)

		for _, key := range keys {
			require.Equal(t, status, key.Status)
		}
	}
}

func TestStorageKeyOrdering(t *testing.T) {
	first := types.ID{
		Owner:    testOwner(1).String(),
		Height:   1,
		Sequence: 1,
	}

	last := types.ID{
		Owner:    testOwner(255).String(),
		Height:   math.MaxInt64,
		Sequence: math.MaxUint64,
	}

	recordIDs := []types.ID{
		first,
		{
			Owner:    first.Owner,
			Height:   1,
			Sequence: math.MaxUint64,
		},
		{
			Owner:    first.Owner,
			Height:   2,
			Sequence: 1,
		},
		{
			Owner:    testOwner(2).String(),
			Height:   1,
			Sequence: 1,
		},
		last,
	}

	chronoIDs := []types.ID{
		first,
		{
			Owner:    last.Owner,
			Height:   1,
			Sequence: 1,
		},
		{
			Owner:    first.Owner,
			Height:   1,
			Sequence: 2,
		},
		{
			Owner:    first.Owner,
			Height:   2,
			Sequence: 1,
		},
		last,
	}
	for _, tc := range []struct {
		codec collcodec.KeyCodec[types.ID]
		ids   []types.ID
	}{
		{RecordIDKey, recordIDs},
		{chronoIDKey, chronoIDs},
	} {
		var previous []byte
		for _, id := range tc.ids {
			encoded := make([]byte, tc.codec.Size(id))
			_, err := tc.codec.Encode(encoded, id)
			require.NoError(t, err)
			require.Negative(t, bytes.Compare(previous, encoded))
			previous = encoded
		}
	}

	var previous []byte
	for status := types.StatusTransferPending; status <= types.StatusRefunded; status++ {
		for _, id := range []types.ID{first, last} {
			key, err := SettlementKey(*types.NewSettlementID(status, id))
			require.NoError(t, err)
			require.Negative(t, bytes.Compare(previous, key))
			previous = key
		}
	}

	refundCodec := collections.TripleKeyCodec(collections.Uint64Key, collections.Int64Key, RecordIDKey)
	previous = nil
	for _, key := range []collections.Triple[uint64, int64, types.ID]{
		collections.Join3(uint64(1), int64(1), first),
		collections.Join3(uint64(1), int64(1), last),
		collections.Join3(uint64(1), int64(2), first),
		collections.Join3(uint64(2), int64(1), first),
	} {
		encoded := make([]byte, refundCodec.Size(key))
		n, err := refundCodec.Encode(encoded, key)
		require.NoError(t, err)
		require.Equal(t, len(encoded), n)
		require.Negative(t, bytes.Compare(previous, encoded))
		consumed, got, err := refundCodec.Decode(encoded)
		require.NoError(t, err)
		require.Equal(t, n, consumed)
		require.Equal(t, key, got)
		previous = encoded
	}

	for _, bad := range []types.ID{
		{},
		{
			Owner:    first.Owner,
			Height:   0,
			Sequence: 1,
		},
		{
			Owner:    first.Owner,
			Height:   1,
			Sequence: 0,
		},
	} {
		_, err := TransferKey(bad)
		require.Error(t, err)
		_, err = SettlementKey(*types.NewSettlementID(types.StatusMinted, bad))
		require.Error(t, err)
	}
}

func TestRefundHeadEqualDeadlineUsesOwnerFirstID(t *testing.T) {
	k, ctx, _ := testStore(t)
	later := types.ID{
		Owner:    testOwner(2).String(),
		Height:   1,
		Sequence: 1,
	}

	earlier := types.ID{
		Owner:    testOwner(1).String(),
		Height:   100,
		Sequence: 100,
	}

	require.NoError(t, k.refunds.Set(ctx, collections.Join3(types.GovernanceRefundScope, int64(100), later)))
	require.NoError(t, k.refunds.Set(ctx, collections.Join3(uint64(1), int64(100), earlier)))
	head, found, err := k.refundHead(ctx, 1)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, earlier, head)
	require.NoError(t, k.refunds.Set(ctx, collections.Join3(types.GovernanceRefundScope, int64(99), later)))
	head, found, err = k.refundHead(ctx, 1)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, later, head)
}

func TestQueryCursorBytesAndFilters(t *testing.T) {
	for _, length := range []int{20, 32, 255} {
		t.Run(strconv.Itoa(length), func(t *testing.T) {
			testQueryCursorBytesAndFilters(t, length)
		})
	}
}

func testQueryCursorBytesAndFilters(t *testing.T, length int) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	owners := map[string]string{
		testOwner(6).String(): sdk.AccAddress(bytes.Repeat([]byte{6}, length)).String(),
		testOwner(7).String(): sdk.AccAddress(bytes.Repeat([]byte{7}, length)).String(),
	}
	for i := range g.Records {
		if owner, ok := owners[g.Records[i].ID.Owner]; ok {
			g.Records[i].ID.Owner = owner
		}
	}
	for i := range g.Accounts {
		if owner, ok := owners[g.Accounts[i].Owner]; ok {
			g.Accounts[i].Owner = owner
		}
	}
	importTestGenesis(t, k, ctx, bank, g)
	q := k.NewQuerier()
	for _, filter := range []types.QueryTransfersRequest{
		{},
		{Owner: owners[testOwner(6).String()]},
		{Status: types.StatusTransferPending},
		{
			Status: types.StatusRefundPending,
			Owner:  owners[testOwner(7).String()],
		},
	} {
		type cursorRecord struct {
			id  types.ID
			key []byte
		}

		var expected []cursorRecord
		for _, record := range g.Records {
			if filter.Owner != "" && record.ID.Owner != filter.Owner || filter.Status != types.StatusInvalid && record.Status != filter.Status {
				continue
			}

			owner, err := sdk.AccAddressFromBech32(record.ID.Owner)
			require.NoError(t, err)
			identity := append([]byte{byte(len(owner))}, owner...)                 //nolint:gosec // Native address lengths fit one byte.
			height := binary.BigEndian.AppendUint64(nil, uint64(record.ID.Height)) //nolint:gosec // Fixture heights are positive.
			sequence := binary.BigEndian.AppendUint64(nil, record.ID.Sequence)
			var key []byte
			if filter.Owner == "" && filter.Status == types.StatusInvalid {
				key = append(append(height, sequence...), identity...)
			} else {
				key = append(append(identity, height...), sequence...)
				if filter.Status != types.StatusInvalid {
					key = append([]byte{byte(record.Status)}, key...) //nolint:gosec // Fixture statuses are 1 through 5.
				}
			}

			expected = append(expected, cursorRecord{
				id:  record.ID,
				key: key,
			})
		}

		slices.SortFunc(expected, func(a, b cursorRecord) int { return bytes.Compare(a.key, b.key) })
		require.GreaterOrEqual(t, len(expected), 2)

		filter.Owner = strings.ToUpper(filter.Owner)
		for _, reverse := range []bool{false, true} {
			if reverse {
				slices.Reverse(expected)
			}

			var cursor []byte
			for i, want := range expected {
				filter.Pagination = &sdkquery.PageRequest{
					Key:     cursor,
					Limit:   1,
					Reverse: reverse,
				}
				page, err := q.Transfers(ctx, &filter)
				require.NoError(t, err)
				require.Len(t, page.Transfers, 1)
				require.Equal(t, want.id, page.Transfers[0].ID)
				cursor = page.Pagination.NextKey
				if i+1 < len(expected) {
					require.Equal(t, expected[i+1].key, cursor)
				} else {
					require.Empty(t, cursor)
				}
			}

			bad := append(bytes.Clone(expected[0].key), 0)
			filter.Pagination = &sdkquery.PageRequest{Key: bad}
			_, err := q.Transfers(ctx, &filter)
			require.Error(t, err)
		}
	}
}

func TestIDCodecsUseNativeAddressSemantics(t *testing.T) {
	for _, length := range []int{0, 1, 19, 20, 21, 32, 255} {
		for _, fill := range []byte{0, 1, 255} {
			raw := sdk.AccAddress(bytes.Repeat([]byte{fill}, length))
			owner := raw.String()
			_, nativeErr := sdk.AccAddressFromBech32(owner)
			id := types.ID{
				Owner:    owner,
				Height:   1,
				Sequence: 1,
			}
			encodedOwner := append([]byte{byte(length)}, raw...) //nolint:gosec // Fixture lengths fit one byte.
			height := binary.BigEndian.AppendUint64(nil, 1)
			sequence := binary.BigEndian.AppendUint64(nil, 1)
			record := append(append(bytes.Clone(encodedOwner), height...), sequence...)
			chronological := append(append(bytes.Clone(height), sequence...), encodedOwner...)
			n, decoded, err := RecordIDKey.Decode(record)
			require.Equal(t, nativeErr == nil, err == nil, "length %d, fill %d", length, fill)
			ns, settlement, err := SettlementIDKey.Decode(append([]byte{1}, record...))
			require.Equal(t, nativeErr == nil, err == nil)
			nc, chrono, err := chronoIDKey.Decode(chronological)
			require.Equal(t, nativeErr == nil, err == nil)
			if nativeErr != nil {
				continue
			}

			require.Equal(t, len(record), n)
			require.Equal(t, n+1, ns)
			require.Equal(t, len(chronological), nc)
			require.Equal(t, id, decoded)
			require.Equal(t, id, *settlement.GetID())
			require.Equal(t, id, chrono)
			prefix, err := RecordIDKey.ToPrefix(types.ID{Owner: owner})
			require.NoError(t, err)
			require.Equal(t, encodedOwner, prefix)
			key, err := TransferKey(id)
			require.NoError(t, err)
			require.Equal(t, append(bytes.Clone(TransfersKey.Bytes()), record...), key)
			key, err = SettlementKey(*types.NewSettlementID(types.StatusTransferPending, id))
			require.NoError(t, err)
			require.Equal(t, append(append(bytes.Clone(SettlementsKey.Bytes()), 1), record...), key)
		}
	}
}

func TestQueryPaginationBounds(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := historyGenesis(1, settlementIn(types.StatusTransferPending), 10, 105)
	importTestGenesis(t, k, ctx, bank, g)
	q := k.NewQuerier()
	for _, filter := range []types.QueryTransfersRequest{
		{},
		{Owner: g.Records[0].ID.Owner},
		{Status: types.StatusMinted},
	} {
		page, err := q.Transfers(ctx, &filter)
		require.NoError(t, err)
		require.Len(t, page.Transfers, 100)
		require.NotEmpty(t, page.Pagination.NextKey)
		filter.Pagination = &sdkquery.PageRequest{Limit: 500}
		page, err = q.Transfers(ctx, &filter)
		require.NoError(t, err)
		require.Greater(t, len(page.Transfers), 100)
		require.Empty(t, page.Pagination.NextKey)
		for _, pagination := range []*sdkquery.PageRequest{
			{Offset: 1},
			{CountTotal: true},
			{Limit: 501},
		} {
			filter.Pagination = pagination
			_, err = q.Transfers(ctx, &filter)
			require.ErrorIs(t, err, types.ErrInvalidPagination)
			_, err = q.Disputes(ctx, &types.QueryDisputesRequest{Pagination: pagination})
			require.ErrorIs(t, err, types.ErrInvalidPagination)
		}
	}
}
