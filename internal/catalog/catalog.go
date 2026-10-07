package catalog

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/WaltzForLovers/Slate/internal/apperr"
	"github.com/WaltzForLovers/Slate/internal/storage"
)

const searchLimit = 10

type Card struct {
	ID             int64
	ExternalID     int
	Name           string
	Year           int
	EpisodeCount   int
	AverageMinutes int
	Kind           string
	Poster         string
}

type Show struct {
	ID             int
	Name           string
	Year           int
	AverageRuntime int
	Runtime        int
	Poster         string
}

type Episode struct {
	Runtime int
}

type Client interface {
	SearchShows(ctx context.Context, query string) ([]Show, error)
	Episodes(ctx context.Context, showID int) ([]Episode, error)
}

type Service struct {
	db      *storage.DB
	client  Client
	timeout time.Duration
}

func New(db *storage.DB, client Client) *Service {
	return &Service{db: db, client: client, timeout: 5 * time.Second}
}

func KindOf(episodeCount int) string {
	switch {
	case episodeCount == 1:
		return "film"
	case episodeCount > 1:
		return "series"
	default:
		return ""
	}
}

func BuildCard(show Show, episodes []Episode) Card {
	average := show.AverageRuntime
	if average <= 0 {
		average = show.Runtime
	}
	if average <= 0 {
		average = meanRuntime(episodes)
	}
	return Card{
		ExternalID:     show.ID,
		Name:           show.Name,
		Year:           show.Year,
		EpisodeCount:   len(episodes),
		AverageMinutes: average,
		Kind:           KindOf(len(episodes)),
		Poster:         show.Poster,
	}
}

func (s *Service) Search(ctx context.Context, query string) ([]Card, error) {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) < 2 {
		return nil, apperr.Validation("Введите не меньше двух символов.")
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	shows, err := s.client.SearchShows(ctx, query)
	if err != nil {
		return nil, apperr.CatalogUnavailable().WithCause(err)
	}
	if len(shows) == 0 {
		return nil, apperr.TitleNotFound()
	}
	if len(shows) > searchLimit {
		shows = shows[:searchLimit]
	}

	episodes := make([][]Episode, len(shows))
	errs := make([]error, len(shows))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, show := range shows {
		wg.Add(1)
		go func(i, showID int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			episodes[i], errs[i] = s.client.Episodes(ctx, showID)
		}(i, show.ID)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, apperr.CatalogUnavailable().WithCause(err)
		}
	}

	cards := make([]Card, 0, len(shows))
	for i, show := range shows {
		card := BuildCard(show, episodes[i])
		saved, err := s.db.UpsertTitle(ctx, storage.Title{
			ExternalID:     card.ExternalID,
			Name:           card.Name,
			Year:           card.Year,
			EpisodeCount:   card.EpisodeCount,
			AverageMinutes: card.AverageMinutes,
			Poster:         card.Poster,
		})
		if err != nil {
			return nil, apperr.Internal(err)
		}
		card.ID = saved.ID
		cards = append(cards, card)
	}
	return cards, nil
}

func (s *Service) Get(ctx context.Context, id int64) (Card, error) {
	title, err := s.db.TitleByID(ctx, id)
	if errors.Is(err, storage.ErrNotFound) {
		return Card{}, apperr.TitleNotFound()
	}
	if err != nil {
		return Card{}, apperr.Internal(err)
	}
	return Card{
		ID:             title.ID,
		ExternalID:     title.ExternalID,
		Name:           title.Name,
		Year:           title.Year,
		EpisodeCount:   title.EpisodeCount,
		AverageMinutes: title.AverageMinutes,
		Kind:           KindOf(title.EpisodeCount),
		Poster:         title.Poster,
	}, nil
}

func meanRuntime(episodes []Episode) int {
	sum, n := 0, 0
	for _, episode := range episodes {
		if episode.Runtime <= 0 {
			continue
		}
		sum += episode.Runtime
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / n
}
