package main

import (
	"reflect"
	"testing"
)

func TestGoConstructions(t *testing.T) {
	for _, c := range []struct {
		name, src, typ string
		want           []int // lines of the constructions found
	}{
		{"bare type: only the unqualified literal",
			"package p\n\nfunc f() {\n\t_ = T{}\n\t_ = x.T{}\n}\n", "T", []int{4}},
		{"generic instantiations construct their base type",
			"package p\n\nfunc f() {\n\t_ = T[int]{}\n\t_ = &T[int, string]{}\n\t_ = new(T[int])\n\t_ = new(T)\n}\n", "T", []int{4, 5, 6, 7}},
		{"qualified: the package imported under its own name",
			"package q\n\nimport \"example.com/m/lensir\"\n\nfunc f() {\n\t_ = &lensir.Program{}\n\t_ = other.Program{}\n\t_ = Program{}\n}\n", "lensir.Program", []int{6}},
		{"qualified: an alias of the package",
			"package q\n\nimport lp \"example.com/m/lensir\"\n\nfunc f() {\n\t_ = lp.Program{}\n\t_ = lp.Program[int]{}\n\t_ = new(lp.Program)\n}\n", "lensir.Program", []int{6, 7, 8}},
		{"qualified: an alias naming another package is that package",
			"package q\n\nimport lensir \"example.com/m/notlensir\"\n\nfunc f() {\n\t_ = lensir.Program{}\n}\n", "lensir.Program", nil},
		{"qualified: a major-version suffix is not the package name",
			"package q\n\nimport y \"go.yaml.in/yaml/v3\"\n\nfunc f() {\n\t_ = y.Node{}\n}\n", "yaml.Node", []int{6}},
		{"qualified: a bare literal inside the package itself",
			"package lensir\n\nfunc f() {\n\t_ = Program{}\n}\n", "lensir.Program", []int{4}},
		{"qualified: a bare literal in another package is another type",
			"package other\n\nfunc f() {\n\t_ = Program{}\n}\n", "lensir.Program", nil},
	} {
		var got []int
		for _, s := range goConstructions(c.src, c.typ) {
			got = append(got, s.Line)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: constructions of %s at lines %v, want %v", c.name, c.typ, got, c.want)
		}
	}
}
