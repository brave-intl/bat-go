package errors

// IsErrNotFound is a helper method for determining if an error indicates a missing resource
func IsErrNotFound(err error) bool {
	type notFound interface {
		NotFoundError() bool
	}
	te, ok := err.(notFound)
	return ok && te.NotFoundError()
}

// IsErrInvalidDestination is a helper method for determining if an error indicates an invalid destination
func IsErrInvalidDestination(err error) bool {
	type invalidDestination interface {
		InvalidDestination() bool
	}
	te, ok := err.(invalidDestination)
	return ok && te.InvalidDestination()
}

// IsErrInsufficientBalance is a helper method for determining if an error indicates insufficient balance
func IsErrInsufficientBalance(err error) bool {
	type insufficientBalance interface {
		InsufficientBalance() bool
	}
	te, ok := err.(insufficientBalance)
	return ok && te.InsufficientBalance()
}

// IsErrUnauthorized is a helper method for determining if an error indicates the wallet unauthorized
func IsErrUnauthorized(err error) bool {
	type unauthorized interface {
		Unauthorized() bool
	}
	te, ok := err.(unauthorized)
	return ok && te.Unauthorized()
}

// IsErrInvalidSignature is a helper method for determining if an error indicates there was an invalid signature
func IsErrInvalidSignature(err error) bool {
	type invalidSignature interface {
		InvalidSignature() bool
	}
	te, ok := err.(invalidSignature)
	return ok && te.InvalidSignature()
}

// IsErrAlreadyExists is a helper method for determining if an error indicates the resource already exists
func IsErrAlreadyExists(err error) bool {
	type alreadyExists interface {
		AlreadyExistsError() bool
	}
	te, ok := err.(alreadyExists)
	return ok && te.AlreadyExistsError()
}

// IsErrForbidden is a helper method for determining if an error indicates the action is forbidden
func IsErrForbidden(err error) bool {
	type forbidden interface {
		ForbiddenError() bool
	}
	te, ok := err.(forbidden)
	return ok && te.ForbiddenError()
}
