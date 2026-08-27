package main

import "testing"

func TestAddressValidation(t *testing.T) {
	for _, valid := range []string{"127.0.0.1:19081", "localhost:19082", "[::1]:19083"} {
		if err := validateAddress(valid); err != nil {
			t.Errorf("合法地址 %s 被拒绝: %v", valid, err)
		}
	}
	for _, invalid := range []string{"0.0.0.0:19081", "127.0.0.1:0", "localhost:bad"} {
		if validateAddress(invalid) == nil {
			t.Errorf("非法地址 %s 未被拒绝", invalid)
		}
	}
}
