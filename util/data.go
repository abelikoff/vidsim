package util

// Represent boolean as a byte

func BoolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
