package main

import (
	"reflect"
	"testing"
)

// TestThemaVersions covers the spellings of thema's keyed normal form and the
// version-shaped text that must not read as a version key.
func TestThemaVersions(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      []ver
	}{
		{"chained labels", `
l: thema.Lineage & {
	schemas: {
		"0": "0": {
			schema: {init: string}
		}
		"0": "1": {
			schema: {init: string, optional?: int}
		}
	}
	lenses: {
		"0.1->0.0": {v0_1: _, v0_0: {init: v0_1.init}}
	}
}`, []ver{{0, 0}, {0, 1}}},
		{"schemas chained onto its keys", `
l: schemas: "0": "0": {schema: {a: string}}
l: schemas: "1": "0": {schema: {b: int}}
`, []ver{{0, 0}, {1, 0}}},
		{"nested structs", `
l: schemas: {
	"0": {
		"0": {schema: {a: string}}, "1": {schema: {a: string, b?: int}}
	}
	"1": {
		"0": {schema: {b: int}}
	}
}`, []ver{{0, 0}, {0, 1}, {1, 0}}},
		{"each lineage in a file adds its own versions", `
a: schemas: "0": "0": {schema: {a: string}}
b: schemas: {
	"0": "0": {schema: {b: string}}
	"0": "1": {schema: {b: string, c?: int}}
}`, []ver{{0, 0}, {0, 0}, {0, 1}}},
		{"a version declared twice in one lineage counts once", `
l: schemas: "0": "0": {schema: {a: string}}
l: schemas: "0": "0": {schema: {a: =~"x"}}
`, []ver{{0, 0}}},
		{"a postfix alias keeps its label", `
l: schemas: {
	"1"~(K,_): "0": {schema: {a: K}}
}`, []ver{{1, 0}}},
		{"comments, strings and schema contents are not version keys", `
// "9": "0": {
l: schemas: {
	// "8": "0": {
	"0": "0": {
		schema: {
			"7": "0": string
			d: *"\"6\": \"0\": {" | string
			e: """
				"5": "0": {
				"""
			f: #""4": "0": {"#
			g: "\("3": "0")"
		}
	}
}`, []ver{{0, 0}}},
		{"keys that are not version keys", `
l: schemas: {
	"01": "0": {schema: {}}
	"x": "0": {schema: {}}
	"1": "y": {schema: {}}
}`, nil},
		{"pattern constraints, as in thema's own lineage definition", `
#Lineage: {
	let versionKey = "^(0|[1-9][0-9]*)$"
	schemas!: struct.MinFields(1) & {
		[=~versionKey]~(M,_): struct.MinFields(1) & {
			[=~versionKey]~(m,_): {
				_version: [strconv.Atoi(M), strconv.Atoi(m)]
			}
			[!~versionKey]~(K,_): error("\(K) is not a minor version, such as \"0\"")
		}
	}
}`, nil},
		{"a list of version: [M, m] entries declares no keys", `
contract: schemas: [
	{version: [0, 0], schema: {a: string}},
]`, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := versionTuples(c.src, "thema")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}
