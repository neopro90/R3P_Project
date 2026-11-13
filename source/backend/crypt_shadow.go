package main

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"hash"
	"os"
	"strconv"
	"strings"
)

const cryptBase64Alphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// verifyShadowPassword checks username's password against a Unix /etc/shadow file.
// Fails closed if the file cannot be read, user is missing, or account is locked.
func verifyShadowPassword(shadowPath, username, password string) bool {
	if shadowPath == "" || username == "" || password == "" {
		return false
	}
	data, err := os.ReadFile(shadowPath)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.SplitN(line, ":", 3)
		if len(fields) < 2 || fields[0] != username {
			continue
		}
		return verifyCryptHash(password, fields[1])
	}
	return false
}

// verifyCryptHash verifies password against a standard Unix crypt hash ($1$, $5$, $6$).
func verifyCryptHash(password, storedHash string) bool {
	if storedHash == "" || strings.HasPrefix(storedHash, "!") || strings.HasPrefix(storedHash, "*") {
		return false
	}
	var computed string
	switch {
	case strings.HasPrefix(storedHash, "$5$"):
		computed = cryptSHA([]byte(password), storedHash, sha256.New, 32, "$5$")
	case strings.HasPrefix(storedHash, "$6$"):
		computed = cryptSHA([]byte(password), storedHash, sha512.New, 64, "$6$")
	case strings.HasPrefix(storedHash, "$1$"):
		computed = cryptMD5([]byte(password), storedHash)
	default:
		return false
	}
	if computed == "" || len(computed) != len(storedHash) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}

func b64From24Bit(b2, b1, b0 byte, n int) []byte {
	w := (uint32(b2) << 16) | (uint32(b1) << 8) | uint32(b0)
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = cryptBase64Alphabet[w&0x3f]
		w >>= 6
	}
	return out
}

func cryptSHA(key []byte, setting string, newHash func() hash.Hash, blockSize int, prefix string) string {
	rest := strings.TrimPrefix(setting, prefix)
	rounds := 5000
	roundsCustom := false
	if strings.HasPrefix(rest, "rounds=") {
		sep := strings.IndexByte(rest, '$')
		if sep < 0 {
			return ""
		}
		val, err := strconv.Atoi(strings.TrimPrefix(rest[:sep], "rounds="))
		if err != nil {
			return ""
		}
		if val < 1000 {
			val = 1000
		} else if val > 999999999 {
			val = 999999999
		}
		rounds = val
		roundsCustom = true
		rest = rest[sep+1:]
	}
	if sep := strings.IndexByte(rest, '$'); sep >= 0 {
		rest = rest[:sep]
	}
	if len(rest) > 16 {
		rest = rest[:16]
	}
	salt := []byte(rest)

	hA := newHash()
	hA.Write(key)
	hA.Write(salt)

	hB := newHash()
	hB.Write(key)
	hB.Write(salt)
	hB.Write(key)
	b := hB.Sum(nil)

	for cnt := len(key); cnt > 0; cnt -= blockSize {
		if cnt >= blockSize {
			hA.Write(b)
		} else {
			hA.Write(b[:cnt])
		}
	}
	for cnt := len(key); cnt > 0; cnt >>= 1 {
		if cnt&1 != 0 {
			hA.Write(b)
		} else {
			hA.Write(key)
		}
	}
	a := hA.Sum(nil)

	hDP := newHash()
	for range key {
		hDP.Write(key)
	}
	dp := hDP.Sum(nil)
	p := repeatBytes(dp, len(key))

	hDS := newHash()
	for i := 0; i < 16+int(a[0]); i++ {
		hDS.Write(salt)
	}
	ds := hDS.Sum(nil)
	s := repeatBytes(ds, len(salt))

	hC := newHash()
	prev := a
	for r := 0; r < rounds; r++ {
		hC.Reset()
		if r&1 != 0 {
			hC.Write(p)
		} else {
			hC.Write(prev)
		}
		if r%3 != 0 {
			hC.Write(s)
		}
		if r%7 != 0 {
			hC.Write(p)
		}
		if r&1 != 0 {
			hC.Write(prev)
		} else {
			hC.Write(p)
		}
		prev = hC.Sum(prev[:0])
	}

	var out bytes.Buffer
	out.WriteString(prefix)
	if roundsCustom {
		out.WriteString("rounds=")
		out.WriteString(strconv.Itoa(rounds))
		out.WriteByte('$')
	}
	out.Write(salt)
	out.WriteByte('$')

	if blockSize == 32 {
		triplets := [][3]int{
			{0, 10, 20}, {21, 1, 11}, {12, 22, 2}, {3, 13, 23}, {24, 4, 14},
			{15, 25, 5}, {6, 16, 26}, {27, 7, 17}, {18, 28, 8}, {9, 19, 29},
		}
		for _, t := range triplets {
			out.Write(b64From24Bit(prev[t[0]], prev[t[1]], prev[t[2]], 4))
		}
		out.Write(b64From24Bit(0, prev[31], prev[30], 3))
	} else {
		triplets := [][3]int{
			{0, 21, 42}, {22, 43, 1}, {44, 2, 23}, {3, 24, 45}, {25, 46, 4},
			{47, 5, 26}, {6, 27, 48}, {28, 49, 7}, {50, 8, 29}, {9, 30, 51},
			{31, 52, 10}, {53, 11, 32}, {12, 33, 54}, {34, 55, 13}, {56, 14, 35},
			{15, 36, 57}, {37, 58, 16}, {59, 17, 38}, {18, 39, 60}, {40, 61, 19},
			{62, 20, 41},
		}
		for _, t := range triplets {
			out.Write(b64From24Bit(prev[t[0]], prev[t[1]], prev[t[2]], 4))
		}
		out.Write(b64From24Bit(0, 0, prev[63], 2))
	}
	return out.String()
}

func cryptMD5(key []byte, setting string) string {
	rest := strings.TrimPrefix(setting, "$1$")
	if sep := strings.IndexByte(rest, '$'); sep >= 0 {
		rest = rest[:sep]
	}
	if len(rest) > 8 {
		rest = rest[:8]
	}
	salt := []byte(rest)

	hA := md5.New()
	hA.Write(key)
	hA.Write([]byte("$1$"))
	hA.Write(salt)

	hB := md5.New()
	hB.Write(key)
	hB.Write(salt)
	hB.Write(key)
	b := hB.Sum(nil)

	for cnt := len(key); cnt > 0; cnt -= 16 {
		if cnt >= 16 {
			hA.Write(b)
		} else {
			hA.Write(b[:cnt])
		}
	}
	for cnt := len(key); cnt > 0; cnt >>= 1 {
		if cnt&1 != 0 {
			hA.Write([]byte{0})
		} else if len(key) > 0 {
			hA.Write(key[:1])
		}
	}
	prev := hA.Sum(nil)

	hC := md5.New()
	for r := 0; r < 1000; r++ {
		hC.Reset()
		if r&1 != 0 {
			hC.Write(key)
		} else {
			hC.Write(prev)
		}
		if r%3 != 0 {
			hC.Write(salt)
		}
		if r%7 != 0 {
			hC.Write(key)
		}
		if r&1 != 0 {
			hC.Write(prev)
		} else {
			hC.Write(key)
		}
		prev = hC.Sum(prev[:0])
	}

	var out bytes.Buffer
	out.WriteString("$1$")
	out.Write(salt)
	out.WriteByte('$')
	triplets := [][3]int{{0, 6, 12}, {1, 7, 13}, {2, 8, 14}, {3, 9, 15}, {4, 10, 5}}
	for _, t := range triplets {
		out.Write(b64From24Bit(prev[t[0]], prev[t[1]], prev[t[2]], 4))
	}
	out.Write(b64From24Bit(0, 0, prev[11], 2))
	return out.String()
}

func repeatBytes(pattern []byte, totalLen int) []byte {
	if totalLen <= 0 || len(pattern) == 0 {
		return nil
	}
	out := make([]byte, totalLen)
	for off := 0; off < totalLen; {
		n := copy(out[off:], pattern)
		off += n
	}
	return out
}
