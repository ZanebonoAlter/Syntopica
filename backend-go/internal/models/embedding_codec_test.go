package models

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddingCodecRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   [][]float64
	}{
		{name: "single vector", in: [][]float64{{0.5, -0.25, 0.125}}},
		{name: "multiple vectors", in: [][]float64{{1, 2}, {3, 4, 5}, {}}},
		{name: "empty payload", in: nil},
		{name: "float32-exact values survive exactly", in: [][]float64{{0.5, -0.75, 1.5}}},
		{name: "negative zero", in: [][]float64{{math.Copysign(0, -1)}}},
		{name: "negative values", in: [][]float64{{-0.1, -0.2, -0.3}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enc := EncodeEmbeddingVectors(tc.in)
			got, err := DecodeEmbeddingVectors(enc)
			require.NoError(t, err)
			require.Equal(t, len(tc.in), len(got))
			for i := range tc.in {
				require.Equal(t, len(tc.in[i]), len(got[i]))
				for j := range tc.in[i] {
					// NaN never compares equal; assert NaN-ness instead.
					if math.IsNaN(tc.in[i][j]) {
						require.True(t, math.IsNaN(got[i][j]))
						continue
					}
					require.InDelta(t, tc.in[i][j], got[i][j], 1e-7, "vec[%d][%d]", i, j)
				}
			}
		})
	}
}

func TestEmbeddingCodecNaNAndFloat32Precision(t *testing.T) {
	enc := EncodeEmbeddingVectors([][]float64{{math.NaN(), math.Inf(1)}})
	got, err := DecodeEmbeddingVectors(enc)
	require.NoError(t, err)
	require.True(t, math.IsNaN(got[0][0]))
	require.True(t, math.IsInf(got[0][1], 1))

	// A float64 value beyond float32 precision round-trips to its float32
	// rounding: end-to-end the pipeline stores vectors as pgvector float32
	// anyway, so this is the pipeline's true precision contract.
	prec := 0.1 + 1e-12
	enc = EncodeEmbeddingVectors([][]float64{{prec}})
	got, err = DecodeEmbeddingVectors(enc)
	require.NoError(t, err)
	require.InDelta(t, prec, got[0][0], 1e-8)
}

func TestEmbeddingCodecSizeMatchesExpectation(t *testing.T) {
	vec := make([]float64, 2560)
	for i := range vec {
		vec[i] = float64(i) * 0.001
	}
	enc := EncodeEmbeddingVectors([][]float64{vec})
	// 8B count + 8B dim + 2560*4B float32 = 10256 bytes (vs ~31.5KB jsonb text).
	require.Equal(t, 8+8+2560*4, len(enc))
}

func TestEmbeddingCodecRejectsTruncatedPayload(t *testing.T) {
	valid := EncodeEmbeddingVectors([][]float64{{0.5, 0.6}})

	for _, tc := range []struct {
		name  string
		input []byte
	}{
		{name: "empty is valid (nil vectors)", input: nil},
		{name: "count header truncated", input: valid[:5]},
		{name: "dim header truncated", input: valid[:12]},
		{name: "float bytes truncated", input: valid[:20]},
		{name: "trailing garbage", input: append(append([]byte{}, valid...), 0x00)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeEmbeddingVectors(tc.input)
			if err != nil {
				require.Nil(t, got)
				return
			}
			require.Empty(t, got)
		})
	}
}
