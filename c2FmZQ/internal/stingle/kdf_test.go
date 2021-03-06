//
// Copyright 2021-2022 TTBT Enterprises LLC
//
// This file is part of c2FmZQ (https://c2FmZQ.org/).
//
// c2FmZQ is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version.
//
// c2FmZQ is distributed in the hope that it will be useful, but WITHOUT ANY
// WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR
// A PARTICULAR PURPOSE. See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with
// c2FmZQ. If not, see <https://www.gnu.org/licenses/>.

package stingle

import (
	"reflect"
	"testing"
)

func TestDeriveKey(t *testing.T) {
	mk := []byte{
		0x4d, 0xe, 0x21, 0xd5, 0x92, 0x6d, 0x45, 0xd1, 0x63, 0x9, 0x9f, 0x7b, 0xe, 0xa4, 0xb8, 0xdf,
		0x5f, 0x95, 0x11, 0xce, 0x7f, 0xd4, 0xc1, 0x3e, 0x98, 0x8f, 0x5c, 0xee, 0xd, 0xaa, 0x8d, 0xba,
	}
	ck := [][]byte{
		[]byte{
			0xc3, 0x2a, 0x8, 0xb2, 0x61, 0x7d, 0xfd, 0xf6, 0x50, 0x84, 0x77, 0x2b, 0xa9, 0xc9, 0xeb, 0xaa,
			0x4d, 0x36, 0xb6, 0xdf, 0xb3, 0x94, 0xe9, 0x64, 0x73, 0x18, 0x21, 0xca, 0x71, 0xe, 0x86, 0xa9,
		},
		[]byte{
			0xc1, 0xd8, 0x70, 0x61, 0x2a, 0xd, 0x4b, 0x53, 0x6, 0xdd, 0x15, 0xf8, 0xde, 0xa7, 0x9d, 0x6e,
			0x95, 0xe6, 0xf6, 0x2e, 0x10, 0x46, 0x6b, 0x72, 0x31, 0x60, 0x3b, 0x3, 0x9c, 0xf8, 0x4e, 0x94,
		},
		[]byte{
			0x94, 0xdb, 0x84, 0xc5, 0x52, 0x20, 0x64, 0xbb, 0x37, 0x7f, 0x69, 0xc6, 0x3e, 0xae, 0xda, 0x22,
			0x9c, 0xbd, 0x4, 0xb0, 0xf4, 0x6e, 0xde, 0xe9, 0x3d, 0x2c, 0x5a, 0xa, 0xbc, 0x3d, 0xfb, 0xd7,
		},
		[]byte{
			0xd6, 0x7e, 0xc9, 0x26, 0xb3, 0x4c, 0xe0, 0x39, 0xa5, 0x3e, 0x27, 0xd1, 0xb3, 0x44, 0x32, 0x1d,
			0x44, 0x21, 0xbe, 0x22, 0x22, 0x47, 0x43, 0x5b, 0x83, 0x94, 0x7c, 0x7f, 0xf8, 0xfc, 0x3b, 0x7d,
		},
		[]byte{
			0x4e, 0x12, 0x7c, 0x25, 0x19, 0x1f, 0x1d, 0x85, 0x45, 0x6c, 0x76, 0x45, 0x23, 0x33, 0x7a, 0xfa,
			0x0, 0xb7, 0x72, 0xbb, 0x84, 0x1a, 0xd9, 0x7, 0x1f, 0xa, 0x67, 0xa5, 0x8b, 0x2b, 0x8d, 0x2b,
		},
		[]byte{
			0x58, 0x16, 0x13, 0x43, 0x6e, 0x17, 0xd7, 0x3d, 0xe5, 0x37, 0x6c, 0x78, 0xbc, 0x83, 0xba, 0x32,
			0x98, 0xcb, 0x87, 0x27, 0xe3, 0xf5, 0x1e, 0x6a, 0xcf, 0x6c, 0xbe, 0xd, 0x6d, 0xfd, 0x2, 0x8,
		},
		[]byte{
			0xa, 0x45, 0x14, 0x81, 0x16, 0xc0, 0x95, 0xa, 0xa8, 0xa8, 0x6d, 0x2c, 0x5b, 0x1c, 0xf7, 0x94,
			0x2e, 0x3d, 0x9d, 0x66, 0xbd, 0x4, 0x4d, 0xd5, 0x36, 0xb2, 0xdd, 0xd, 0x70, 0x29, 0x9e, 0x91,
		},
		[]byte{
			0x47, 0xa1, 0x88, 0x8b, 0x93, 0xc0, 0xa7, 0xbe, 0x2a, 0x8e, 0xfa, 0xa2, 0x3d, 0x76, 0x83, 0xfa,
			0x50, 0x67, 0x3d, 0x92, 0x5f, 0x72, 0x8c, 0x4d, 0xab, 0xf8, 0x5, 0x98, 0x12, 0xba, 0x17, 0x11,
		},
		[]byte{
			0x54, 0xbf, 0x82, 0x44, 0xa5, 0x2c, 0x48, 0xdd, 0x70, 0xeb, 0x18, 0xaf, 0xd, 0xd7, 0xf9, 0xf2,
			0xd4, 0x98, 0xb6, 0xdc, 0x1, 0x5d, 0x29, 0x9c, 0x38, 0x82, 0xd6, 0x3b, 0x67, 0x29, 0xf4, 0x7e,
		},
		[]byte{
			0xe7, 0x74, 0xfe, 0xa2, 0xdd, 0xc8, 0xa9, 0x36, 0xb6, 0xb, 0x77, 0x3b, 0x1f, 0x13, 0xb9, 0x99,
			0xbd, 0xd9, 0xcd, 0x9b, 0xf5, 0x3b, 0x52, 0xcf, 0xc1, 0x2e, 0x7c, 0xf9, 0x6c, 0x8a, 0x9a, 0x95,
		},
	}

	for i := range ck {
		dk := DeriveKey(mk, 32, uint64(i+1), "__data__")
		if want, got := ck[i], dk; !reflect.DeepEqual(want, got) {
			t.Fatalf("[%d] derived key mismatch. Want %v, got %v", i+1, want, got)
		}
	}
}
