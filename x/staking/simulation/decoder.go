package simulation

// NewDecodeStore returns a decoder function closure that unmarshals the KVPair's
// Value to the corresponding mint type.
// func NewDecodeStore(_ codec.Codec) func(kvA, kvB kv.Pair) string {
// 	return func(kvA, kvB kv.Pair) string {
// 		switch {
// 		// case bytes.Equal(kvA.Key, types.MinterKey):
// 		// 	var minterA, minterB types.Minter
// 		// 	cdc.MustUnmarshal(kvA.Value, &minterA)
// 		// 	cdc.MustUnmarshal(kvB.Value, &minterB)
// 		// 	return fmt.Sprintf("%v\n%v", minterA, minterB)
// 		default:
// 			panic(fmt.Sprintf("invalid astaking key %X", kvA.Key))
// 		}
// 	}
// }