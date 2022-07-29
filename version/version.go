//go:build dev
// +build dev

package version

import (
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

var (
	letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	// Version is the version number of the program.
	Version = randSeq(16)

	// Revision is the git revision that program was built from.
	Revision = randSeq(16)

	// Branch is the git branch that program was built from.
	Branch = "dev"

	// BuildUser is the user that built program.
	BuildUser = "dev"

	// BuildHost is the host that built program.
	BuildHost = "dev"

	// BuildDate is the date that program was built.
	BuildDate = time.Now().String()
)
