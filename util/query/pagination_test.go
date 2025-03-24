package query

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodePaginationKey(t *testing.T) {
	type testCase struct {
		name        string
		states      []byte
		prefix      []byte
		key         []byte
		unsolicited []byte
		wantOutput  []byte
		wantErr     bool
	}
	tests := []testCase{
		{
			name:        "fail/all params empty",
			states:      nil,
			prefix:      nil,
			key:         nil,
			unsolicited: nil,
			wantOutput:  nil,
			wantErr:     true,
		},
		{
			name:        "fail/key is empty",
			states:      []byte{1},
			prefix:      []byte{2},
			key:         nil,
			unsolicited: nil,
			wantOutput:  nil,
			wantErr:     true,
		},
		{
			name:        "fail/prefix is empty",
			states:      []byte{1},
			prefix:      nil,
			key:         []byte{3},
			unsolicited: nil,
			wantOutput:  nil,
			wantErr:     true,
		},
		{
			name:        "fail/states is empty",
			states:      nil,
			prefix:      []byte{2},
			key:         []byte{3},
			unsolicited: nil,
			wantOutput:  nil,
			wantErr:     true,
		},
		{
			name:        "pass/all params valid",
			states:      []byte{1},
			prefix:      []byte{2},
			key:         []byte{3},
			unsolicited: nil,
			wantOutput:  []byte{0x7c, 0xd4, 0x88, 0x46, 0x1, 0x1, 0x1, 0x2, 0x1, 0x3},
			wantErr:     false,
		},
		{
			name:        "pass/all params valid with unsolicited",
			states:      []byte{1},
			prefix:      []byte{2},
			key:         []byte{3},
			unsolicited: []byte{4},
			wantOutput:  []byte{0x1a, 0xef, 0x78, 0xe2, 0x1, 0x1, 0x1, 0x2, 0x1, 0x3, 0x1, 0x4},
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOutput, err := EncodePaginationKey(tt.states, tt.prefix, tt.key, tt.unsolicited)
			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, gotOutput)
			} else {
				require.NoError(t, err)
				require.NotNil(t, gotOutput)
				require.Equal(t, tt.wantOutput, gotOutput)
			}
		})
	}
}

func TestDecodePaginationKey(t *testing.T) {
	type testCase struct {
		name          string
		input         []byte
		wantStates    []byte
		wantPrefix    []byte
		wantKey       []byte
		wantUnsol     []byte
		wantErr       bool
		wantErrString string
	}

	tests := []testCase{
		{
			name:    "fail/too short key",
			input:   []byte{0x01, 0x02, 0x03, 0x04},
			wantErr: true,
		},
		{
			name:          "fail/invalid checksum",
			input:         []byte{0x01, 0x02, 0x03, 0x04, 0x01, 65},
			wantErr:       true,
			wantErrString: "pagination: invalid key: invalid checksum, 0x01020304 != 0x591952b8",
		},
		{
			name:          "fail/invalid states length",
			input:         []byte{0xA5, 0x05, 0xDF, 0x1B, 1},
			wantErr:       true,
			wantErrString: "pagination: invalid key: invalid state length",
		},
		{
			name:          "fail/invalid prefix length",
			input:         []byte{0x90, 0x9F, 0xB2, 0xF2, 0x01, 0x01, 0x01},
			wantErr:       true,
			wantErrString: "pagination: invalid key: invalid state length",
		},
		{
			name:          "fail/invalid key length",
			input:         []byte{0x07, 0x0D, 0x81, 0xEB, 0x01, 0x01, 0x01, 0x02, 0x01},
			wantErr:       true,
			wantErrString: "pagination: invalid key: invalid state length",
		},
		{
			name:          "fail/invalid unsolicited length",
			input:         []byte{0x3A, 0xC6, 0xEF, 0x36, 0x01, 0x01, 0x01, 0x02, 0x01, 0x03, 0x01},
			wantErr:       true,
			wantErrString: "pagination: invalid key: invalid state length",
		},
		{
			name:          "pass/without unsolicited",
			input:         makeTestKey(t, []byte{1}, []byte{2}, []byte{3}, nil),
			wantStates:    []byte{1},
			wantPrefix:    []byte{2},
			wantKey:       []byte{3},
			wantUnsol:     nil,
			wantErr:       false,
			wantErrString: "",
		},
		{
			name:          "pass/key with unsolicited",
			input:         makeTestKey(t, []byte{1}, []byte{2}, []byte{3}, []byte{4}),
			wantStates:    []byte{1},
			wantPrefix:    []byte{2},
			wantKey:       []byte{3},
			wantUnsol:     []byte{4},
			wantErr:       false,
			wantErrString: "",
		},
		{
			name:          "pass/multiple states",
			input:         makeTestKey(t, []byte{1, 7}, []byte{2}, []byte{3}, nil),
			wantStates:    []byte{1, 7},
			wantPrefix:    []byte{2},
			wantKey:       []byte{3},
			wantUnsol:     nil,
			wantErr:       false,
			wantErrString: "",
		},
		{
			name:          "pass/key with multiple bytes",
			input:         makeTestKey(t, []byte{1, 7}, []byte{2, 29, 1}, []byte{3}, nil),
			wantStates:    []byte{1, 7},
			wantPrefix:    []byte{2, 29, 1},
			wantKey:       []byte{3},
			wantUnsol:     nil,
			wantErr:       false,
			wantErrString: "",
		},
		{
			name:          "pass/unsolicited with multiple bytes",
			input:         makeTestKey(t, []byte{1, 7}, []byte{2, 29, 1}, []byte{3, 2, 17}, nil),
			wantStates:    []byte{1, 7},
			wantPrefix:    []byte{2, 29, 1},
			wantKey:       []byte{3, 2, 17},
			wantUnsol:     nil,
			wantErr:       false,
			wantErrString: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStates, gotPrefix, gotKey, gotUnsol, err := DecodePaginationKey(tt.input)

			if tt.wantErr {
				require.Error(t, err, "DecodePaginationKey() expected error but got none")

				if tt.wantErrString != "" {
					require.Equal(t, tt.wantErrString, err.Error(), "DecodePaginationKey() unexpected error string")
				}

				require.Nil(t, gotStates, "DecodePaginationKey() expected states to be nil")
				require.Nil(t, gotPrefix, "DecodePaginationKey() expected prefix to be nil")
				require.Nil(t, gotKey, "DecodePaginationKey() expected key to be nil")
				require.Nil(t, gotUnsol, "DecodePaginationKey() expected unsolicited to be nil")

				return
			}

			require.NoError(t, err, "DecodePaginationKey() unexpected error")

			require.Equal(t, tt.wantStates, gotStates, "DecodePaginationKey() unexpected states")
			require.Equal(t, tt.wantPrefix, gotPrefix, "DecodePaginationKey() unexpected prefix")
			require.Equal(t, tt.wantKey, gotKey, "DecodePaginationKey() unexpected key")
			require.Equal(t, tt.wantUnsol, gotUnsol, "DecodePaginationKey() unexpected unsolicited")
		})
	}
}

// makeTestKey is a helper function to create a valid pagination key for testing
func makeTestKey(t *testing.T, states, prefix, key, unsolicited []byte) []byte {
	if len(states) == 0 {
		t.Fatal("states cannot be empty")
	}
	if len(prefix) == 0 {
		t.Fatal("prefix cannot be empty")
	}
	if len(key) == 0 {
		t.Fatal("key cannot be empty")
	}

	encoded, err := EncodePaginationKey(states, prefix, key, unsolicited)
	if err != nil {
		t.Fatalf("failed to encode pagination key: %v", err)
	}

	return encoded
}
