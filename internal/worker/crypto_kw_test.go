package worker

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// RFC 3394 Test Vectors from Appendix A
// https://www.rfc-editor.org/rfc/rfc3394

// TestAESKeyWrap_RFC3394_A1 tests 128-bit KEK wrapping 128-bit key data
func TestAESKeyWrap_RFC3394_A1(t *testing.T) {
	kek, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F")
	plaintext, _ := hex.DecodeString("00112233445566778899AABBCCDDEEFF")
	expected, _ := hex.DecodeString("1FA68B0A8112B447AEF34BD8FB5A7B829D3E862371D2CFE5")

	wrapped, err := aesKeyWrap(kek, plaintext)
	if err != nil {
		t.Fatalf("aesKeyWrap failed: %v", err)
	}
	if !bytes.Equal(wrapped, expected) {
		t.Errorf("wrapped mismatch:\ngot:  %x\nwant: %x", wrapped, expected)
	}
}

// TestAESKeyUnwrap_RFC3394_A1 tests 128-bit KEK unwrapping 128-bit key data
func TestAESKeyUnwrap_RFC3394_A1(t *testing.T) {
	kek, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F")
	ciphertext, _ := hex.DecodeString("1FA68B0A8112B447AEF34BD8FB5A7B829D3E862371D2CFE5")
	expected, _ := hex.DecodeString("00112233445566778899AABBCCDDEEFF")

	unwrapped, err := aesKeyUnwrap(kek, ciphertext)
	if err != nil {
		t.Fatalf("aesKeyUnwrap failed: %v", err)
	}
	if !bytes.Equal(unwrapped, expected) {
		t.Errorf("unwrapped mismatch:\ngot:  %x\nwant: %x", unwrapped, expected)
	}
}

// TestAESKeyWrap_RFC3394_A2 tests 192-bit KEK wrapping 128-bit key data
func TestAESKeyWrap_RFC3394_A2(t *testing.T) {
	kek, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F1011121314151617")
	plaintext, _ := hex.DecodeString("00112233445566778899AABBCCDDEEFF")
	expected, _ := hex.DecodeString("96778B25AE6CA435F92B5B97C050AED2468AB8A17AD84E5D")

	wrapped, err := aesKeyWrap(kek, plaintext)
	if err != nil {
		t.Fatalf("aesKeyWrap failed: %v", err)
	}
	if !bytes.Equal(wrapped, expected) {
		t.Errorf("wrapped mismatch:\ngot:  %x\nwant: %x", wrapped, expected)
	}
}

// TestAESKeyUnwrap_RFC3394_A2 tests 192-bit KEK unwrapping 128-bit key data
func TestAESKeyUnwrap_RFC3394_A2(t *testing.T) {
	kek, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F1011121314151617")
	ciphertext, _ := hex.DecodeString("96778B25AE6CA435F92B5B97C050AED2468AB8A17AD84E5D")
	expected, _ := hex.DecodeString("00112233445566778899AABBCCDDEEFF")

	unwrapped, err := aesKeyUnwrap(kek, ciphertext)
	if err != nil {
		t.Fatalf("aesKeyUnwrap failed: %v", err)
	}
	if !bytes.Equal(unwrapped, expected) {
		t.Errorf("unwrapped mismatch:\ngot:  %x\nwant: %x", unwrapped, expected)
	}
}

// TestAESKeyWrap_RFC3394_A3 tests 256-bit KEK wrapping 128-bit key data
func TestAESKeyWrap_RFC3394_A3(t *testing.T) {
	kek, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F")
	plaintext, _ := hex.DecodeString("00112233445566778899AABBCCDDEEFF")
	expected, _ := hex.DecodeString("64E8C3F9CE0F5BA263E9777905818A2A93C8191E7D6E8AE7")

	wrapped, err := aesKeyWrap(kek, plaintext)
	if err != nil {
		t.Fatalf("aesKeyWrap failed: %v", err)
	}
	if !bytes.Equal(wrapped, expected) {
		t.Errorf("wrapped mismatch:\ngot:  %x\nwant: %x", wrapped, expected)
	}
}

// TestAESKeyUnwrap_RFC3394_A3 tests 256-bit KEK unwrapping 128-bit key data
func TestAESKeyUnwrap_RFC3394_A3(t *testing.T) {
	kek, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F")
	ciphertext, _ := hex.DecodeString("64E8C3F9CE0F5BA263E9777905818A2A93C8191E7D6E8AE7")
	expected, _ := hex.DecodeString("00112233445566778899AABBCCDDEEFF")

	unwrapped, err := aesKeyUnwrap(kek, ciphertext)
	if err != nil {
		t.Fatalf("aesKeyUnwrap failed: %v", err)
	}
	if !bytes.Equal(unwrapped, expected) {
		t.Errorf("unwrapped mismatch:\ngot:  %x\nwant: %x", unwrapped, expected)
	}
}

// TestAESKeyWrap_RFC3394_A4 tests 192-bit KEK wrapping 192-bit key data
func TestAESKeyWrap_RFC3394_A4(t *testing.T) {
	kek, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F1011121314151617")
	plaintext, _ := hex.DecodeString("00112233445566778899AABBCCDDEEFF0001020304050607")
	expected, _ := hex.DecodeString("031D33264E15D33268F24EC260743EDCE1C6C7DDEE725A936BA814915C6762D2")

	wrapped, err := aesKeyWrap(kek, plaintext)
	if err != nil {
		t.Fatalf("aesKeyWrap failed: %v", err)
	}
	if !bytes.Equal(wrapped, expected) {
		t.Errorf("wrapped mismatch:\ngot:  %x\nwant: %x", wrapped, expected)
	}
}

// TestAESKeyUnwrap_RFC3394_A4 tests 192-bit KEK unwrapping 192-bit key data
func TestAESKeyUnwrap_RFC3394_A4(t *testing.T) {
	kek, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F1011121314151617")
	ciphertext, _ := hex.DecodeString("031D33264E15D33268F24EC260743EDCE1C6C7DDEE725A936BA814915C6762D2")
	expected, _ := hex.DecodeString("00112233445566778899AABBCCDDEEFF0001020304050607")

	unwrapped, err := aesKeyUnwrap(kek, ciphertext)
	if err != nil {
		t.Fatalf("aesKeyUnwrap failed: %v", err)
	}
	if !bytes.Equal(unwrapped, expected) {
		t.Errorf("unwrapped mismatch:\ngot:  %x\nwant: %x", unwrapped, expected)
	}
}

// TestAESKeyWrap_RFC3394_A5 tests 256-bit KEK wrapping 192-bit key data
func TestAESKeyWrap_RFC3394_A5(t *testing.T) {
	kek, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F")
	plaintext, _ := hex.DecodeString("00112233445566778899AABBCCDDEEFF0001020304050607")
	expected, _ := hex.DecodeString("A8F9BC1612C68B3FF6E6F4FBE30E71E4769C8B80A32CB8958CD5D17D6B254DA1")

	wrapped, err := aesKeyWrap(kek, plaintext)
	if err != nil {
		t.Fatalf("aesKeyWrap failed: %v", err)
	}
	if !bytes.Equal(wrapped, expected) {
		t.Errorf("wrapped mismatch:\ngot:  %x\nwant: %x", wrapped, expected)
	}
}

// TestAESKeyUnwrap_RFC3394_A5 tests 256-bit KEK unwrapping 192-bit key data
func TestAESKeyUnwrap_RFC3394_A5(t *testing.T) {
	kek, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F")
	ciphertext, _ := hex.DecodeString("A8F9BC1612C68B3FF6E6F4FBE30E71E4769C8B80A32CB8958CD5D17D6B254DA1")
	expected, _ := hex.DecodeString("00112233445566778899AABBCCDDEEFF0001020304050607")

	unwrapped, err := aesKeyUnwrap(kek, ciphertext)
	if err != nil {
		t.Fatalf("aesKeyUnwrap failed: %v", err)
	}
	if !bytes.Equal(unwrapped, expected) {
		t.Errorf("unwrapped mismatch:\ngot:  %x\nwant: %x", unwrapped, expected)
	}
}

// TestAESKeyWrap_RFC3394_A6 tests 256-bit KEK wrapping 256-bit key data
func TestAESKeyWrap_RFC3394_A6(t *testing.T) {
	kek, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F")
	plaintext, _ := hex.DecodeString("00112233445566778899AABBCCDDEEFF000102030405060708090A0B0C0D0E0F")
	expected, _ := hex.DecodeString("28C9F404C4B810F4CBCCB35CFB87F8263F5786E2D80ED326CBC7F0E71A99F43BFB988B9B7A02DD21")

	wrapped, err := aesKeyWrap(kek, plaintext)
	if err != nil {
		t.Fatalf("aesKeyWrap failed: %v", err)
	}
	if !bytes.Equal(wrapped, expected) {
		t.Errorf("wrapped mismatch:\ngot:  %x\nwant: %x", wrapped, expected)
	}
}

// TestAESKeyUnwrap_RFC3394_A6 tests 256-bit KEK unwrapping 256-bit key data
func TestAESKeyUnwrap_RFC3394_A6(t *testing.T) {
	kek, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F")
	ciphertext, _ := hex.DecodeString("28C9F404C4B810F4CBCCB35CFB87F8263F5786E2D80ED326CBC7F0E71A99F43BFB988B9B7A02DD21")
	expected, _ := hex.DecodeString("00112233445566778899AABBCCDDEEFF000102030405060708090A0B0C0D0E0F")

	unwrapped, err := aesKeyUnwrap(kek, ciphertext)
	if err != nil {
		t.Fatalf("aesKeyUnwrap failed: %v", err)
	}
	if !bytes.Equal(unwrapped, expected) {
		t.Errorf("unwrapped mismatch:\ngot:  %x\nwant: %x", unwrapped, expected)
	}
}

// TestAESKeyWrap_Roundtrip tests wrap then unwrap produces original plaintext
func TestAESKeyWrap_Roundtrip(t *testing.T) {
	tests := []struct {
		name      string
		kekSize   int
		plainSize int
	}{
		{"128-bit KEK, 16-byte plain", 16, 16},
		{"128-bit KEK, 24-byte plain", 16, 24},
		{"128-bit KEK, 32-byte plain", 16, 32},
		{"192-bit KEK, 16-byte plain", 24, 16},
		{"192-bit KEK, 32-byte plain", 24, 32},
		{"256-bit KEK, 16-byte plain", 32, 16},
		{"256-bit KEK, 24-byte plain", 32, 24},
		{"256-bit KEK, 64-byte plain", 32, 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kek := make([]byte, tt.kekSize)
			for i := range kek {
				kek[i] = byte(i)
			}

			plaintext := make([]byte, tt.plainSize)
			for i := range plaintext {
				plaintext[i] = byte(i * 2)
			}

			wrapped, err := aesKeyWrap(kek, plaintext)
			if err != nil {
				t.Fatalf("aesKeyWrap failed: %v", err)
			}

			unwrapped, err := aesKeyUnwrap(kek, wrapped)
			if err != nil {
				t.Fatalf("aesKeyUnwrap failed: %v", err)
			}

			if !bytes.Equal(unwrapped, plaintext) {
				t.Errorf("roundtrip failed:\noriginal: %x\nrecovered: %x", plaintext, unwrapped)
			}
		})
	}
}

// TestAESKeyWrap_LargePlaintext tests wrapping a larger plaintext (64 bytes = 8 blocks)
func TestAESKeyWrap_LargePlaintext(t *testing.T) {
	kek, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F")
	plaintext := make([]byte, 64)
	for i := range plaintext {
		plaintext[i] = byte(i)
	}

	wrapped, err := aesKeyWrap(kek, plaintext)
	if err != nil {
		t.Fatalf("aesKeyWrap failed: %v", err)
	}

	// Verify wrapped is 8 bytes longer (the IV)
	if len(wrapped) != len(plaintext)+8 {
		t.Errorf("wrapped length = %d, want %d", len(wrapped), len(plaintext)+8)
	}

	unwrapped, err := aesKeyUnwrap(kek, wrapped)
	if err != nil {
		t.Fatalf("aesKeyUnwrap failed: %v", err)
	}

	if !bytes.Equal(unwrapped, plaintext) {
		t.Errorf("large plaintext roundtrip failed")
	}
}

// TestAESKeyWrap_ErrorPlaintextNotMultipleOf8 tests error when plaintext is not a multiple of 8 bytes
func TestAESKeyWrap_ErrorPlaintextNotMultipleOf8(t *testing.T) {
	kek := make([]byte, 16)
	tests := []int{1, 7, 15, 17, 23, 25, 31}

	for _, size := range tests {
		t.Run(string(rune(size))+" bytes", func(t *testing.T) {
			plaintext := make([]byte, size)
			_, err := aesKeyWrap(kek, plaintext)
			if err == nil {
				t.Errorf("expected error for plaintext size %d, got nil", size)
			}
		})
	}
}

// TestAESKeyWrap_ErrorPlaintextTooShort tests error when plaintext is less than 16 bytes
func TestAESKeyWrap_ErrorPlaintextTooShort(t *testing.T) {
	kek := make([]byte, 16)
	plaintext := make([]byte, 8) // 8 bytes is multiple of 8 but too short

	_, err := aesKeyWrap(kek, plaintext)
	if err == nil {
		t.Error("expected error for plaintext < 16 bytes, got nil")
	}
}

// TestAESKeyWrap_ErrorInvalidKEKLength tests error when KEK has invalid AES key length
func TestAESKeyWrap_ErrorInvalidKEKLength(t *testing.T) {
	tests := []int{8, 15, 17, 23, 25, 31, 33}

	for _, kekSize := range tests {
		t.Run(string(rune(kekSize))+" byte KEK", func(t *testing.T) {
			kek := make([]byte, kekSize)
			plaintext := make([]byte, 16)
			_, err := aesKeyWrap(kek, plaintext)
			if err == nil {
				t.Errorf("expected error for KEK size %d, got nil", kekSize)
			}
		})
	}
}

// TestAESKeyUnwrap_ErrorCiphertextNotMultipleOf8 tests error when ciphertext is not a multiple of 8 bytes
func TestAESKeyUnwrap_ErrorCiphertextNotMultipleOf8(t *testing.T) {
	kek := make([]byte, 16)
	tests := []int{1, 7, 23, 25, 31}

	for _, size := range tests {
		t.Run(string(rune(size))+" bytes", func(t *testing.T) {
			ciphertext := make([]byte, size)
			_, err := aesKeyUnwrap(kek, ciphertext)
			if err == nil {
				t.Errorf("expected error for ciphertext size %d, got nil", size)
			}
		})
	}
}

// TestAESKeyUnwrap_ErrorCiphertextTooShort tests error when ciphertext is less than 24 bytes
func TestAESKeyUnwrap_ErrorCiphertextTooShort(t *testing.T) {
	kek := make([]byte, 16)
	tests := []int{8, 16}

	for _, size := range tests {
		t.Run(string(rune(size))+" bytes", func(t *testing.T) {
			ciphertext := make([]byte, size)
			_, err := aesKeyUnwrap(kek, ciphertext)
			if err == nil {
				t.Errorf("expected error for ciphertext size %d, got nil", size)
			}
		})
	}
}

// TestAESKeyUnwrap_ErrorWrongKey tests integrity check failure when unwrapping with wrong key
func TestAESKeyUnwrap_ErrorWrongKey(t *testing.T) {
	kek1, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F")
	kek2, _ := hex.DecodeString("0F0E0D0C0B0A09080706050403020100") // Different key
	plaintext, _ := hex.DecodeString("00112233445566778899AABBCCDDEEFF")

	wrapped, err := aesKeyWrap(kek1, plaintext)
	if err != nil {
		t.Fatalf("aesKeyWrap failed: %v", err)
	}

	// Try to unwrap with wrong key
	_, err = aesKeyUnwrap(kek2, wrapped)
	if err == nil {
		t.Error("expected integrity check error when unwrapping with wrong key, got nil")
	}
	if err != nil && err.Error() != "AES-KW unwrap: integrity check failed" {
		t.Errorf("expected integrity check error, got: %v", err)
	}
}

// TestAESKeyUnwrap_ErrorInvalidKEKLength tests error when KEK has invalid AES key length
func TestAESKeyUnwrap_ErrorInvalidKEKLength(t *testing.T) {
	tests := []int{8, 15, 17, 23, 25, 31, 33}

	for _, kekSize := range tests {
		t.Run(string(rune(kekSize))+" byte KEK", func(t *testing.T) {
			kek := make([]byte, kekSize)
			ciphertext := make([]byte, 24)
			_, err := aesKeyUnwrap(kek, ciphertext)
			if err == nil {
				t.Errorf("expected error for KEK size %d, got nil", kekSize)
			}
		})
	}
}

// TestAESKeyUnwrap_ErrorCorruptedCiphertext tests integrity check failure with corrupted ciphertext
func TestAESKeyUnwrap_ErrorCorruptedCiphertext(t *testing.T) {
	kek, _ := hex.DecodeString("000102030405060708090A0B0C0D0E0F")
	plaintext, _ := hex.DecodeString("00112233445566778899AABBCCDDEEFF")

	wrapped, err := aesKeyWrap(kek, plaintext)
	if err != nil {
		t.Fatalf("aesKeyWrap failed: %v", err)
	}

	// Corrupt a byte in the middle
	wrapped[10] ^= 0x01

	_, err = aesKeyUnwrap(kek, wrapped)
	if err == nil {
		t.Error("expected integrity check error for corrupted ciphertext, got nil")
	}
}

// TestAESKeyWrap_AllKeySizes tests all valid AES key sizes
func TestAESKeyWrap_AllKeySizes(t *testing.T) {
	tests := []struct {
		name    string
		kekSize int
	}{
		{"AES-128", 16},
		{"AES-192", 24},
		{"AES-256", 32},
	}

	plaintext := make([]byte, 32)
	for i := range plaintext {
		plaintext[i] = byte(i)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kek := make([]byte, tt.kekSize)
			for i := range kek {
				kek[i] = byte(i * 3)
			}

			wrapped, err := aesKeyWrap(kek, plaintext)
			if err != nil {
				t.Fatalf("aesKeyWrap failed: %v", err)
			}

			unwrapped, err := aesKeyUnwrap(kek, wrapped)
			if err != nil {
				t.Fatalf("aesKeyUnwrap failed: %v", err)
			}

			if !bytes.Equal(unwrapped, plaintext) {
				t.Errorf("roundtrip failed for %s", tt.name)
			}
		})
	}
}
