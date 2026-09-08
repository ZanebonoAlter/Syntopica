package models

import (
	"encoding/binary"
	"errors"
	"math"
)

// embeddingCacheCodec: ai_embedding_cache.embedding stores vectors as a
// compact little-endian byte stream instead of jsonb text (jsonb floating
// point text cost ~31.5KB per 2560-dim row; binary costs ~10KB).
//
// Layout: [8B vector count][per vector: 8B dim + dim*4B float32 LE].
// Values are stored as float32: provider embeddings are float32 in origin
// (JSON roundtrip aside), and every downstream consumer persists to
// pgvector's float32 vector storage, so float32 here is the end-to-end
// precision of the pipeline.
//
// An empty byte slice decodes to zero vectors (nil, nil); it is a valid
// payload for a cache row written with no vectors.

var errCodecTruncated = errors.New("embedding codec: truncated payload")

func EncodeEmbeddingVectors(vectors [][]float64) []byte {
	size := 8
	for _, v := range vectors {
		size += 8 + len(v)*4
	}
	buf := make([]byte, 0, size)
	buf = binary.LittleEndian.AppendUint64(buf, uint64(len(vectors)))
	for _, v := range vectors {
		buf = binary.LittleEndian.AppendUint64(buf, uint64(len(v)))
		for _, f := range v {
			buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(float32(f)))
		}
	}
	return buf
}

func DecodeEmbeddingVectors(b []byte) ([][]float64, error) {
	if len(b) == 0 {
		return nil, nil
	}
	if len(b) < 8 {
		return nil, errCodecTruncated
	}
	countU := binary.LittleEndian.Uint64(b[:8])
	if countU > uint64(len(b)/8) { // each vector needs at least its 8B dim header
		return nil, errCodecTruncated
	}
	count := int(countU) // #nosec G115 -- bounded by len(b)/8 above, cannot overflow int
	out := make([][]float64, 0, count)
	pos := 8
	for i := 0; i < count; i++ {
		if pos+8 > len(b) {
			return nil, errCodecTruncated
		}
		dimU := binary.LittleEndian.Uint64(b[pos : pos+8])
		pos += 8
		remaining := uint64((len(b) - pos) / 4) // #nosec G115 -- len(b) >= pos here, non-negative int widened safely
		if dimU > remaining {                   // dim*4 float32 bytes must fit in the rest
			return nil, errCodecTruncated
		}
		dim := int(dimU) // #nosec G115 -- bounded by remaining bytes above, cannot overflow int
		vec := make([]float64, dim)
		for j := 0; j < dim; j++ {
			vec[j] = float64(math.Float32frombits(binary.LittleEndian.Uint32(b[pos : pos+4])))
			pos += 4
		}
		out = append(out, vec)
	}
	if pos != len(b) { // trailing garbage means the payload was not written by this codec
		return nil, errCodecTruncated
	}
	return out, nil
}
