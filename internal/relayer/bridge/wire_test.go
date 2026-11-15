package bridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/header"
)

type wireFixture struct {
	Curves []struct {
		Hex     string
		OnCurve bool
	}
	Owner        string
	Height       string
	Sequence     string
	NativeKey    string
	SourceHashes map[string]string
	Addresses    map[string]string
	Accounts     map[string]string
	Instructions map[string]struct {
		Program string
		Data    string
		Keys    []struct {
			Key      string
			Signer   bool
			Writable bool
		}
	}
}

func fixtures(t testing.TB) wireFixture {
	t.Helper()
	data, err := os.ReadFile("testdata/wire.json")
	require.NoError(t, err)
	var f wireFixture
	require.NoError(t, json.Unmarshal(data, &f))
	return f
}

func keyByte(v byte) solana.PublicKey {
	var key solana.PublicKey
	for i := range key {
		key[i] = v
	}
	return key
}

func unhex(t testing.TB, value string) []byte {
	t.Helper()
	data, err := hex.DecodeString(value)
	require.NoError(t, err)
	return data
}

func fixtureID(t testing.TB) sink.ID {
	t.Helper()
	f := fixtures(t)
	height, err := strconv.ParseInt(f.Height, 10, 64)
	require.NoError(t, err)

	sequence, err := strconv.ParseUint(f.Sequence, 10, 64)
	require.NoError(t, err)
	return sink.ID{
		Owner:    f.Owner,
		Height:   height,
		Sequence: sequence,
	}
}

func fixtureClient() ClientState {
	return ClientState{
		ChainID:               "akash-local-1",
		TrustLevelNumerator:   1,
		TrustLevelDenominator: 3,
		TrustingPeriod:        math.MaxUint64,
		UnbondingPeriod:       1814400,
		MaxClockDrift:         10,
		LatestHeight: Height{
			RevisionNumber: 1,
			RevisionHeight: 20,
		},
	}
}

func TestUnsignedWireOracle(t *testing.T) {
	golden := fixtures(t)
	d, err := Derive(keyByte(1))
	require.NoError(t, err)

	f, err := DeriveClient(keyByte(2), keyByte(3))
	require.NoError(t, err)
	f.Latest = 20
	// Resolution proofs use height 19 so swapping latest/proof metas cannot pass.
	require.NotEqual(t, golden.Addresses["new_consensus_state_store"], golden.Addresses["trusted_consensus_state"])
	payer, recipient := keyByte(4), keyByte(5)
	session := [32]byte(keyByte(6))
	id := fixtureID(t)
	nativeKey := unhex(t, golden.NativeKey)
	buffer, err := d.ProofBuffer(payer, nativeKey)
	require.NoError(t, err)

	chunk, err := f.ChunkAt(payer, 20, 0)
	require.NoError(t, err)

	batch, err := f.BatchAt(payer, session)
	require.NoError(t, err)

	receipt := header.Receipt{
		Hash:    keyByte(9),
		PubKey:  keyByte(10),
		Message: []byte{11, 12, 13},
	}
	for i := range receipt.Signature {
		receipt.Signature[i] = 14
	}
	packet := header.Packet{
		Timestamp:          math.MaxUint64,
		Root:               keyByte(7),
		NextValidatorsHash: keyByte(8),
	}
	checked := map[string]bool{}
	check := func(name string) func(solana.Instruction, error) {
		return func(instruction solana.Instruction, err error) {
			t.Helper()
			require.NoError(t, err, name)
			want, exists := golden.Instructions[name]
			require.True(t, exists, name)
			data, err := instruction.Data()
			require.NoError(t, err, name)
			require.Equal(t, want.Data, hex.EncodeToString(data), name)
			require.Equal(t, want.Program, instruction.ProgramID().String(), name)
			metas := instruction.Accounts()
			require.Len(t, metas, len(want.Keys), name)
			for i, expected := range want.Keys {
				require.Equal(t, expected.Key, metas[i].PublicKey.String(), "%s account %d", name, i)
				require.Equal(t, expected.Signer, metas[i].IsSigner, "%s signer %d", name, i)
				require.Equal(t, expected.Writable, metas[i].IsWritable, "%s writable %d", name, i)
			}
			checked[name] = true
		}
	}
	check("create_proof_buffer")(d.CreateProofBuffer(payer, nativeKey, 1234))
	check("grow_proof_buffer")(d.GrowProofBuffer(payer, buffer))
	check("append_proof")(d.AppendProof(payer, buffer, 450, []byte{1, 2, 3}))
	check("close_proof_buffer")(d.CloseProofBuffer(payer, buffer))
	check("mint_transfer")(d.MintTransfer(MintArgs{
		Payer:       payer,
		Client:      f,
		ID:          id,
		ProofBuffer: buffer,
		Recipient:   recipient,
		Amount:      math.MaxUint64,
		ProofHeight: 19,
	}))
	check("void_transfer")(d.VoidTransfer(VoidArgs{
		Payer:            payer,
		Client:           f,
		ID:               id,
		ProofBuffer:      buffer,
		SettlementStatus: 5,
		ProofHeight:      19,
	}))
	check("signature-create")(f.InitializeSignatureBatch(payer, session))
	check("signature")(f.PreVerifySignatureBatch(payer, batch, receipt))
	check("chunk")(f.UploadHeaderChunk(payer, 20, 0, []byte{1, 2, 3}))
	check("assembly")(f.Assemble(payer, 20, 19, []solana.PublicKey{chunk}, []solana.PublicKey{batch}))
	check("cleanup")(f.Cleanup(payer, []solana.PublicKey{chunk, batch}))
	check("prune")(f.Prune(payer, 20))
	check("client-initialize")(f.Initialize(payer, payer, fixtureClient(), packet))
	check("access-initialize")(InitializeAccess(f.AccessManager, payer, payer, payer))
	check("grant-role")(GrantRole(f.AccessManager, payer, payer, 1))
	check("initialize")(d.Initialize(payer))
	check("configure_bridge")(d.ConfigureBridge(payer, f, "akash-local-1", MintPolicy{
		PerTransferMax: 1000,
		DailyMintCap:   10000,
		Vaults:         []solana.PublicKey{recipient},
	}))
	check("set_bridge_enabled")(d.SetBridgeEnabled(payer, true))
	check("ed25519")(Ed25519(receipt))
	compute := Compute()
	check("compute-limit")(compute[0], nil)
	check("compute-heap")(compute[1], nil)
	create, lookup, err := CreateLookup(payer, payer, 123)
	check("lookup-create")(create, err)
	require.Equal(t, golden.Addresses["lookup"], lookup.String())
	check("lookup-extend")(ExtendLookup(lookup, payer, payer, []solana.PublicKey{chunk, batch}))
	check("lookup-deactivate")(DeactivateLookup(lookup, payer))
	check("lookup-close")(CloseLookup(lookup, payer, payer))
	require.Len(t, checked, len(golden.Instructions))
}

func TestPDAsAndCanonicalIdentity(t *testing.T) {
	g := fixtures(t)
	d, err := Derive(keyByte(1))
	require.NoError(t, err)

	f, err := DeriveClient(keyByte(2), keyByte(3))
	require.NoError(t, err)

	for name, key := range map[string]solana.PublicKey{
		"config":         d.Config,
		"bridge":         d.Bridge,
		"mint":           d.Mint,
		"mint_authority": d.MintAuthority,
		"client_state":   f.ClientState,
		"app_state":      f.AppState,
		"access_manager": f.AccessState,
	} {
		require.Equal(t, g.Addresses[name], key.String())
	}
	id := fixtureID(t)
	address, err := d.Status(id)
	require.NoError(t, err)
	require.Equal(t, g.Addresses["status"], address.String())

	changed := id
	changed.Sequence++
	other, err := d.Status(changed)
	require.NoError(t, err)
	require.NotEqual(t, address, other)

	for _, bad := range []sink.ID{
		{
			Owner:    id.Owner,
			Height:   0,
			Sequence: 1,
		},
		{
			Owner:    id.Owner,
			Height:   1,
			Sequence: 0,
		},
		{
			Owner:    "bad",
			Height:   1,
			Sequence: 1,
		},
	} {
		_, err := d.Status(bad)
		require.Error(t, err)
	}
}

func TestWireOracleSourcePins(t *testing.T) {
	hashes := fixtures(t).SourceHashes
	for _, name := range []string{"mint_transfer", "void_transfer", "proof_buffer", "genesis", "bridge_admin", "upgrade_authority"} {
		require.Contains(t, hashes, "contracts/solana/programs/akt/src/instructions/"+name+".rs")
	}
	for path, want := range hashes {
		data, err := os.ReadFile(filepath.Join("../../..", path))
		require.NoError(t, err)
		hash := sha256.Sum256(data)
		require.Equal(t, want, hex.EncodeToString(hash[:]), path)
	}
}

func TestCompileV0AndLoadedAccountLimit(t *testing.T) {
	payer, program := keyByte(4), keyByte(1)
	for _, withTable := range []bool{false, true} {
		for _, count := range []int{62, 63} {
			accounts := solana.AccountMetaSlice{}
			addresses := solana.PublicKeySlice{}
			for i := 0; i < count; i++ {
				key := solana.PublicKey(sha256.Sum256([]byte(strconv.Itoa(i))))
				accounts = append(accounts, solana.Meta(key).WRITE())
				addresses = append(addresses, key)
			}
			var tables map[solana.PublicKey]solana.PublicKeySlice
			if withTable {
				tables = map[solana.PublicKey]solana.PublicKeySlice{keyByte(20): addresses}
			}
			instruction := solana.NewInstruction(program, accounts, []byte{1})
			tx, n, err := Compile(payer, solana.Hash(keyByte(7)), []solana.Instruction{instruction}, tables)
			require.Equal(t, count+2, n)
			if count == 63 {
				require.Error(t, err)
				require.Nil(t, tx)
				continue
			}
			require.NoError(t, err)
			data, err := tx.Message.MarshalBinary()
			require.NoError(t, err)
			require.Equal(t, byte(0x80), data[0])
			require.Empty(t, tx.Signatures)
			keys, err := tx.Message.GetAllKeys()
			require.NoError(t, err)
			for i, index := range tx.Message.Instructions[0].Accounts {
				require.Equal(t, addresses[i], keys[index])
			}
			if withTable {
				require.Equal(t, count, tx.Message.AddressTableLookups.NumLookups())
			}
		}
	}
	_, _, err := Compile(payer, solana.Hash{}, nil, nil)
	require.Error(t, err)
}

func TestBuilderWireBounds(t *testing.T) {
	_, err := Ed25519(header.Receipt{Message: bytes.Repeat([]byte{1}, math.MaxUint16+1)})
	require.Error(t, err)

	f, err := DeriveClient(keyByte(2), keyByte(3))
	require.NoError(t, err)
	_, err = f.Assemble(keyByte(4), 20, 19, nil, nil)
	require.Error(t, err)
	_, err = f.Assemble(keyByte(4), 20, 19, make([]solana.PublicKey, 256), nil)
	require.Error(t, err)
}

func TestCanonicalCurveOracle(t *testing.T) {
	for _, curve := range fixtures(t).Curves {
		key := solana.PublicKeyFromBytes(unhex(t, curve.Hex))
		require.Equal(t, curve.OnCurve, canonicalOnCurve(key), curve.Hex)
	}
}
