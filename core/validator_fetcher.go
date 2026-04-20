package core

func (v *Validator) IsRequestPending(hash string) bool {
	return v.pendingMgr.IsPending(hash)
}

func (v *Validator) AddPendingRequest(hash string) {
	v.pendingMgr.Add(hash)
}

func (v *Validator) RemovePendingRequest(hash string) {
	v.pendingMgr.Remove(hash)
}
