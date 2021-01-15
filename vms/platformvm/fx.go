package platformvm

// Fx is the interface a feature extension must implement to support the
// Platform Chain.
type Fx interface {
	// Initialize this feature extension to be running under this VM. Should
	// return an error if the VM is incompatible.
	Initialize(vm interface{}) error

	// Notify this Fx that the VM is in bootstrapping
	Bootstrapping() error

	// Notify this Fx that the VM is bootstrapped
	Bootstrapped() error

	// VerifyTransfer returns nil iff the given input and output are well-formed
	// and syntactically valid. Does not check signatures. That should
	// be done by calling VerifyPermission.
	VerifyTransfer(in, out interface{}) error

	// VerifyPermission returns nil iff [cred] proves that [owner]
	// assents to [tx]
	VerifyPermission(tx, in, cred, owner interface{}) error

	// CreateOutput creates a new output with the provided owner worth
	// the specified amount
	CreateOutput(amount uint64, owner interface{}) (interface{}, error)
}

// Owned ...
type Owned interface {
	Owners() interface{}
}
