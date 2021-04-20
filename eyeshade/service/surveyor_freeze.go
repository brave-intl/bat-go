package eyeshade

import (
	"errors"
	"time"

	"github.com/brave-intl/bat-go/eyeshade/models"
)

// FreezeSurveyors freezes surveyors that can be frozen
// given an optional lag time (usualy 1 day)
func (service *Service) FreezeSurveyors(days ...int) error {
	lag := 1 // only relevant for non virtual
	if len(days) > 0 {
		lag = days[0]
	}
	ds := service.Datastore()
	ctx, _, err := ds.ResolveConnection(service.Context())
	if err != nil {
		return err
	}
	defer ds.Rollback(ctx)
	surveyors, err := ds.GetFreezableSurveyors(ctx, lag)
	if err != nil {
		return err
	}
	surveyorIDs := []string{}
	for _, surveyor := range *surveyors {
		surveyorIDs = append(surveyorIDs, surveyor.ID)
	}
	frozenSurveyors, err := ds.FreezeSurveyors(ctx, surveyorIDs...)
	if err != nil {
		return err
	}

	frozenSurveyorIDs := []string{}
	surveyorIDToCreatedAt := map[string]*time.Time{}
	for _, surveyor := range *frozenSurveyors {
		frozenSurveyorIDs = append(frozenSurveyorIDs, surveyor.ID)
		surveyorIDToCreatedAt[surveyor.ID] = &surveyor.CreatedAt
	}
	if err := ds.SetVoteFees(ctx, frozenSurveyorIDs...); err != nil {
		return err
	}
	ballots, err := ds.CountBallots(ctx, frozenSurveyorIDs...)
	if err != nil {
		return err
	}
	for i, ballot := range *ballots {
		ballot.SurveyorCreatedAt = surveyorIDToCreatedAt[ballot.SurveyorID]
		if ballot.SurveyorCreatedAt == nil {
			return errors.New("unable to match ballot to surveyor")
		}
		(*ballots)[i] = ballot
	}

	convertables := models.BallotsToConvertableTransactions(
		*ballots...,
	)
	if err := ds.InsertConvertableTransactions(ctx, convertables); err != nil {
		return err
	}

	return ds.Commit(ctx)
}
