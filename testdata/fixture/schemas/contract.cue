package fixture

// A versioned contract (thema-shaped: the author declares version tuples).
contract: {
	schemas: [
		{version: [0, 0], schema: {a: string}},
		{version: [1, 0], schema: {b: int}},
	]
}
