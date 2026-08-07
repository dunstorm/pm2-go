package server

func (api *Handler) bumpOperationGenerationLocked(processId int32) int64 {
	if api.operationGen == nil {
		api.operationGen = make(map[int32]int64)
	}
	api.nextOperationGen++
	api.operationGen[processId] = api.nextOperationGen
	return api.nextOperationGen
}

func (api *Handler) operationGenerationLocked(processId int32) int64 {
	if api.operationGen == nil {
		return 0
	}
	return api.operationGen[processId]
}

func (api *Handler) beginOperationLocked(processId int32) (int64, bool) {
	if api.operationActive == nil {
		api.operationActive = make(map[int32]bool)
	}
	if api.operationActive[processId] {
		return 0, false
	}
	api.operationActive[processId] = true
	return api.bumpOperationGenerationLocked(processId), true
}

func (api *Handler) finishOperationLocked(processId int32) {
	if api.operationActive != nil {
		delete(api.operationActive, processId)
	}
}

func (api *Handler) clearOperationGenerationLocked(processId int32) {
	if api.operationGen != nil {
		delete(api.operationGen, processId)
	}
	api.finishOperationLocked(processId)
}
