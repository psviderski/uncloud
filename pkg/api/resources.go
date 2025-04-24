package api

type CPUResources struct {
	// Limit is the maximum amount of CPU nanocores (1000000000 = 1 CPU core) the container can use.
	Limit int64
}

type MemoryResources struct {
	// Limit is the maximum amount of memory (in bytes) the container can use.
	Limit int64
	// Reservation is the minimum amount of memory (in bytes) the container needs to run efficiently.
	// TODO: implement a placement constraint that checks available memory on machines.
	Reservation int64
}
