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

func (api *Handler) clearOperationGenerationLocked(processId int32) {
	if api.operationGen != nil {
		delete(api.operationGen, processId)
	}
}
