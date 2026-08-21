package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if valid, err := VerifyPassword("correct horse battery staple", hash); err != nil || !valid {
		t.Fatalf("expected valid password, valid=%v err=%v", valid, err)
	}
	if valid, err := VerifyPassword("incorrect", hash); err != nil || valid {
		t.Fatalf("expected invalid password, valid=%v err=%v", valid, err)
	}
}
