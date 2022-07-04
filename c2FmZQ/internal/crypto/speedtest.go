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

package crypto

import (
	"io"
	"time"

	"c2FmZQ/internal/log"
)

// Fastest runs an in-memory speedtest and returns the fastest encryption
// algorithm on the local computer.
func Fastest() (int, error) {
	algos := []struct {
		name string
		alg  int
		mk   func() (MasterKey, error)
	}{
		{"AES256", AES256, CreateAESMasterKey},
		{"Chacha20Poly1305", Chacha20Poly1305, CreateChacha20Poly1305MasterKey},
	}
	var fastest int = -1
	var fastestName string
	var fastestTime time.Duration
	mb := 20
	for _, a := range algos {
		mk, err := a.mk()
		if err != nil {
			return 0, err
		}
		t, err := speedTest(mk, mb<<20)
		mk.Wipe()
		if err != nil {
			return 0, err
		}
		log.Debugf("speedtest: %s(%d) encrypted %d MiB in %s", a.name, a.alg, mb, t)
		if fastest == -1 || t < fastestTime {
			fastest = a.alg
			fastestName = a.name
			fastestTime = t
		}
	}
	log.Infof("Using %s encryption.", fastestName)
	return fastest, nil
}

func speedTest(mk MasterKey, size int) (d time.Duration, err error) {
	start := time.Now()
	w, err := mk.StartWriter(nil, io.Discard)
	if err != nil {
		return d, err
	}
	var buf [4096]byte
	for size > 0 {
		n := size
		if n > len(buf) {
			n = len(buf)
		}
		if _, err := w.Write(buf[:n]); err != nil {
			return d, err
		}
		size -= n
	}
	if err := w.Close(); err != nil {
		return d, err
	}
	return time.Since(start), nil
}
