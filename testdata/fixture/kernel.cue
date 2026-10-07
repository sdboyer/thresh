package fixture

// The co-owned kernel: definitions both the CUE and Go sides stand on.
#Kernel: {
	name: string
	items: [...#Item]
	lenses: [...#Lens]
}

#Item: {
	schema: _
}

#Lens: {
	from: string
	to:   string
}
