package fixture

// Closed shapes, kept as definitions on purpose; nothing else here should be.
#Shape: {
	name: string
	edges: [...#Edge]
}

#Edge: {
	from: string
	to:   string
}
