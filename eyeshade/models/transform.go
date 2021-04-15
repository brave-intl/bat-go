package models

import (
	"sort"
)

// CollectSurveyorIDs collects surveyor ids from votes
func CollectSurveyorIDs(date string, votes []Vote) ([]Surveyor, []string) {
	surveyorIDsCollected := map[string]bool{}
	surveyorIDs := []string{}
	surveyors := []Surveyor{}
	for _, vote := range votes {
		voteSurveyors := vote.CollectSurveyors(date, surveyorIDsCollected)
		for _, voteSurveyor := range voteSurveyors {
			surveyorIDsCollected[voteSurveyor.ID] = true
			surveyorIDs = append(surveyorIDs, voteSurveyor.ID)
		}
		surveyors = append(surveyors, voteSurveyors...)
	}
	return surveyors, surveyorIDs
}

// SurveyorIDsToFrozen turns a slice of surveyors into a map indicating which
// surveyors are frozen. this might be possible to abstract into a query
func SurveyorIDsToFrozen(insertedSurveyors []Surveyor) map[string]bool {
	frozenSurveyors := map[string]bool{}
	for _, insertedSurveyor := range insertedSurveyors {
		// use frozen surveyors as a filter to not insert votes
		frozenSurveyors[insertedSurveyor.ID] = insertedSurveyor.Frozen
	}
	return frozenSurveyors
}

// CollectBallots collects ballots from vote type
func CollectBallots(
	date string,
	votes []Vote,
	frozenSurveyors map[string]bool,
) ([]Surveyor, []Ballot) {
	surveyorIDsCollected := map[string]bool{}
	ballots := []Ballot{}
	surveyors := []Surveyor{}
	for _, vote := range votes {
		voteSurveyors, voteBallots := vote.CollectBallots(
			date,
			frozenSurveyors,
			surveyorIDsCollected,
		)
		surveyors = append(surveyors, voteSurveyors...)
		ballots = append(ballots, voteBallots...)
		for _, voteSurveyor := range voteSurveyors {
			surveyorIDsCollected[voteSurveyor.ID] = true
		}
	}
	return surveyors, ballots
}

// CondenseBallots returns a list of ballots condensed
// so that fewer db operations can be performed
func CondenseBallots(ballots *[]Ballot, order ...bool) *[]Ballot {
	ids := []string{}
	hash := map[string]*Ballot{}
	for i := range *ballots {
		ballot := (*ballots)[i]
		if hash[ballot.ID] == nil {
			ids = append(ids, ballot.ID)
			hash[ballot.ID] = &ballot
		} else {
			hash[ballot.ID].Tally = hash[ballot.ID].Tally.Add(ballot.Tally)
		}
	}
	if len(order) > 0 && order[0] {
		sort.Strings(ids)
	}

	b := []Ballot{}
	for _, key := range ids {
		b = append(b, *hash[key])
	}
	return &b
}

// CollectTransactionIDs collects a convertable transaction's transaction ids
func CollectTransactionIDs(transactions ...ConvertableTransaction) []string {
	transactionIDs := []string{}
	for _, tx := range transactions {
		transactionIDs = append(transactionIDs, tx.ToTxIDs()...)
	}
	return transactionIDs
}

// CollectBallotIDs collects the ballot ids from a series of votes
func CollectBallotIDs(date string, votes ...Vote) []string {
	voteIDs := []string{}
	hash := map[string]bool{}
	for _, suggestion := range votes {
		ids := suggestion.GetBallotIDs(date)
		for _, id := range ids {
			if !hash[id] {
				hash[id] = true
				voteIDs = append(voteIDs, id)
			}
		}
	}
	return voteIDs
}
