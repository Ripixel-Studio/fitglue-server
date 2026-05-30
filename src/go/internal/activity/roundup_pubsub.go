package activity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

type roundupTriggerMessage struct {
	PeriodType  string `json:"period_type"`
	PeriodStart string `json:"period_start,omitempty"` // optional: YYYY-MM-DD, overrides computed start
	PeriodEnd   string `json:"period_end,omitempty"`   // optional: YYYY-MM-DD, overrides computed end
}

// HandleRoundupTrigger is the HTTP handler for /pubsub/roundup.
func (s *Service) HandleRoundupTrigger(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error(ctx, "roundup trigger: failed to read body", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var envelope struct {
		Message struct {
			Data []byte `json:"data"`
		} `json:"message"`
	}
	msg := body
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Message.Data) > 0 {
		msg = envelope.Message.Data
	}

	var trigger roundupTriggerMessage
	if err := json.Unmarshal(msg, &trigger); err != nil {
		s.logger.Error(ctx, "roundup trigger: failed to parse message", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	periodType, ok := parsePeriodType(trigger.PeriodType)
	if !ok {
		s.logger.Error(ctx, "roundup trigger: unknown period_type", "value", trigger.PeriodType)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	periodStart, periodEnd := RoundupPeriodBounds(periodType, time.Now().UTC())
	if trigger.PeriodStart != "" {
		if t, err := time.Parse("2006-01-02", trigger.PeriodStart); err == nil {
			periodStart = t.UTC()
		} else {
			s.logger.Warn(ctx, "roundup trigger: invalid period_start, using computed", "value", trigger.PeriodStart)
		}
	}
	if trigger.PeriodEnd != "" {
		if t, err := time.Parse("2006-01-02", trigger.PeriodEnd); err == nil {
			periodEnd = t.UTC()
		} else {
			s.logger.Warn(ctx, "roundup trigger: invalid period_end, using computed", "value", trigger.PeriodEnd)
		}
	}
	userIDs, err := s.store.ListAllShowcaseUserIDs(ctx)
	if err != nil {
		s.logger.Error(ctx, "roundup trigger: failed to list users", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.logger.Info(ctx, "roundup trigger: starting generation",
		"period_type", trigger.PeriodType, "user_count", len(userIDs))

	const maxConcurrency = 10
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	for _, uid := range userIDs {
		uid := uid
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			genCtx := context.Background()
			if _, err := s.generateRoundup(genCtx, uid, periodType, periodStart, periodEnd); err != nil {
				s.logger.Warn(genCtx, "roundup generation failed for user", "user_id", uid, "error", err)
			}
		}()
	}
	wg.Wait()
	s.logger.Info(ctx, "roundup trigger: generation complete", "period_type", trigger.PeriodType)
	w.WriteHeader(http.StatusOK)
}

func parsePeriodType(s string) (pbactivity.RoundupPeriodType, bool) {
	switch s {
	case "week":
		return pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_WEEK, true
	case "month":
		return pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_MONTH, true
	case "year":
		return pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_YEAR, true
	}
	return pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_UNSPECIFIED, false
}
