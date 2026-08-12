package auth

import (
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

func TestMatchTOTPStepReturnsActualSkewedCounter(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1_700_000_000, 0).UTC()
	opts := totp.ValidateOpts{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}
	for _, offset := range []int64{-1, 0, 1} {
		at := now.Add(time.Duration(offset*30) * time.Second)
		code, err := totp.GenerateCodeCustom(secret, at, opts)
		if err != nil {
			t.Fatal(err)
		}
		step, valid := MatchTOTPStep(secret, code, now)
		if !valid {
			t.Fatalf("offset %d did not validate", offset)
		}
		want := uint64(at.Unix() / 30)
		if step != want {
			t.Fatalf("offset %d matched step %d, want %d", offset, step, want)
		}
	}
}
