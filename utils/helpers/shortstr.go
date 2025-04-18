package helpers

import (
	"crypto/md5"
	"fmt"
	"math/rand"
	"strconv"
	"time"
)

// @param url
// @return
func short(str string) []string {
	chars := []string{
		"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l",
		"m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x",
		"y", "z", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9",
		"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L",
		"M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X",
		"Y", "Z",
	}

	md5Hash := md5.Sum([]byte(str))
	hex := fmt.Sprintf("%x", md5Hash)

	results := make([]string, 4)

	for i := 0; i < 4; i++ {
		sTempSubString := hex[i*8 : i*8+8]

		lHex, _ := strconv.ParseInt(sTempSubString, 16, 64)

		char := ""

		for j := 0; j < 6; j++ {
			index := int(lHex & 0x0000003D)

			char += chars[index]

			lHex >>= 5
		}
		results[i] = char
	}

	return results
}

func Short(str string) string {
	results := short(str)

	return results[0] + results[1]
}

func ShortUnique(str string) string {
	slat := strconv.FormatInt(time.Now().UnixNano(), 10) + strconv.Itoa(rand.Intn(100000000))
	results := short(slat + str)

	return results[0] + results[1]
}
