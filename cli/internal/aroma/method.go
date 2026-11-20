// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package aroma

// Method is one method of a source file: the unit the algorithm indexes and
// recommends from.
type Method struct {
	// Name is a name for reports, such as Recv.Method.
	Name string
	// StartLine and EndLine are the 1-based lines the method spans.
	StartLine int
	EndLine   int
	// Body is the method's body. Parameters and the name are not part of it.
	Body Node
}
