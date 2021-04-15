package models

// Vote works as an interface for insertions into votes table
// methods help pull out information about how to update votes table and make sure
// surveyor_groups exist
type Vote interface {
	CollectBallots(string, ...map[string]bool) ([]Surveyor, []Ballot)
	CollectSurveyors(string, ...map[string]bool) []Surveyor
	GetBallotIDs(string) []string
}

// ContributionsToVotes turns contributions into votes
func ContributionsToVotes(contributions ...Contribution) []Vote {
	votes := []Vote{}
	for i := range contributions {
		votes = append(votes, &contributions[i])
	}
	return votes
}

// SuggestionsToVotes turns contributions into votes
func SuggestionsToVotes(suggestions ...Suggestion) []Vote {
	votes := []Vote{}
	for i := range suggestions {
		votes = append(votes, &suggestions[i])
	}
	return votes
}
