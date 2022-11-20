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

//go:build !go1.20
// +build !go1.20

package webpush

import (
	"crypto/elliptic"
	"crypto/rand"

	"github.com/aead/ecdh"
)

func ecdhSharedSecret(peerKeyBytes []byte) (secret, publicKey []byte, err error) {
	curve := elliptic.P256()

	x, y := elliptic.Unmarshal(curve, peerKeyBytes)
	peerKey := ecdh.Point{X: x, Y: y}

	p256 := ecdh.Generic(curve)
	localKey, localKeyPub, err := p256.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	pt := localKeyPub.(ecdh.Point)
	localKeyPubBytes := elliptic.Marshal(curve, pt.X, pt.Y)

	sharedSecret := p256.ComputeSecret(localKey, peerKey)
	return sharedSecret, localKeyPubBytes, nil
}
