package sound

import "fmt"

func choosePeriod(requested int, fundamental, minimum, maximum uint32) (uint32, error) {
	if fundamental == 0 || minimum == 0 || maximum < minimum {
		return 0, fmt.Errorf("invalid supported device periods")
	}
	selected := max(uint32(requested), minimum)
	selected = (selected + fundamental - 1) / fundamental * fundamental
	selected = min(selected, maximum/fundamental*fundamental)
	if selected < minimum {
		return 0, fmt.Errorf("no valid device period")
	}
	return selected, nil
}
