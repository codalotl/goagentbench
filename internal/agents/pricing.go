package agents

func calculateLLMCost(llm LLMDefinition, nonCached, cached, writeCached, output int) float64 {
	if llm.Costs == nil || !llm.Costs.IsComplete() {
		return 0
	}
	return float64(nonCached)*(*llm.Costs.InputCost/1_000_000) +
		float64(cached)*(*llm.Costs.CachedInputCost/1_000_000) +
		float64(writeCached)*(cacheWriteInputCost(llm.Costs)/1_000_000) +
		float64(output)*(*llm.Costs.OutputCost/1_000_000)
}

func cacheWriteInputCost(costs *LLMCostDefinition) float64 {
	if costs == nil || costs.CacheWriteInputCost == nil {
		return 0
	}
	return *costs.CacheWriteInputCost
}
