// ==========================================================================
// vidsim - a tool to compare and group large sets of images for similarity.
//
// Copyright © 2024 Alexander L. Belikoff <alexander@belikoff.net>
//
// This software is released under the BSD 3-Clause License. See the LICENSE
// file for details.
//
// https://github.com/abelikoff/vidsim
//
// ==========================================================================

package util

// Represent boolean as a byte

func BoolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
