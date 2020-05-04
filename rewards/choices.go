package rewards

/*
const (
	choiceTable = [][]float64{
		{3, 5, 7, 10, 20},
		{4, 6, 9, 12, 25},
		{5, 8, 11, 17, 35},
		{6, 10, 14, 20, 40},
		{9, 12, 20, 35, 50},
		{15, 25, 35, 50, 100},
		{20, 35, 50, 85},
		{30, 50, 70, 100},
	}
	priceIncrements = []float64{
		1, 0.8, 0.6, 0.5, 0.35, 0.2, 0.15, 0.1,
	}
)

func getChoices(ratio float64) []float64 {
	// find the price increment given our ratio
	var (
		index  int = -1
		result     = []float64{}
	)

	for i, v := range priceIncrements {
		if v <= ratio {
			// this is the index we want from our choice table
			index = i
		}
	}
	if index < 0 || index > len(priceIncrements)-1 {
		// use the last index if no matches
		index = len(priceIncrements) - 1
	}
	return choiceTable[index]
}
*/
