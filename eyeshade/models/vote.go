package models

// Vote works as an interface for insertions into votes table
// methods help pull out information about how to update votes table and make sure
// surveyor_groups exist
type Vote interface {
	CollectBallots(map[string]bool, map[string]bool, string) ([]Surveyor, []Ballot)
	CollectSurveyors(map[string]bool, string) []Surveyor
}
