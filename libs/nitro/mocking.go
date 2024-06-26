package nitro

var enclaveMocking bool

func MockEnclave() {
	enclaveMocking = true
}

func EnclaveMocking() bool {
	return enclaveMocking
}
