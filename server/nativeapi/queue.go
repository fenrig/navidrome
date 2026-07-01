package nativeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Masterminds/squirrel"
	playlistsvc "github.com/navidrome/navidrome/core/playlists"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	. "github.com/navidrome/navidrome/utils/gg"
	"github.com/navidrome/navidrome/utils/slice"
)

type updateQueuePayload struct {
	Ids      *[]string `json:"ids,omitempty"`
	Current  *int      `json:"current,omitempty"`
	Position *int64    `json:"position,omitempty"`
}

type saveQueueAsPlaylistPayload struct {
	Name string `json:"name,omitempty"`
}

type autofillQueuePayload struct {
	Count      int      `json:"count,omitempty"`
	ExcludeIDs []string `json:"excludeIds,omitempty"`
}

// validateCurrentIndex validates that the current index is within bounds of the items array.
// Returns false if validation fails (and sends error response), true if validation passes.
func validateCurrentIndex(w http.ResponseWriter, current int, itemsLength int) bool {
	if current < 0 || current >= itemsLength {
		http.Error(w, "current index out of bounds", http.StatusBadRequest)
		return false
	}
	return true
}

// retrieveExistingQueue retrieves an existing play queue for a user with proper error handling.
// Returns the queue (nil if not found) and false if an error occurred and response was sent.
func retrieveExistingQueue(ctx context.Context, w http.ResponseWriter, ds model.DataStore, userID string) (*model.PlayQueue, bool) {
	existing, err := ds.PlayQueue(ctx).Retrieve(userID)
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		log.Error(ctx, "Error retrieving queue", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, false
	}
	return existing, true
}

// decodeUpdatePayload decodes the JSON payload from the request body.
// Returns false if decoding fails (and sends error response), true if successful.
func decodeUpdatePayload(w http.ResponseWriter, r *http.Request) (*updateQueuePayload, bool) {
	var payload updateQueuePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, false
	}
	return &payload, true
}

// createMediaFileItems converts a slice of IDs to MediaFile items.
func createMediaFileItems(ids []string) []model.MediaFile {
	return slice.Map(ids, func(id string) model.MediaFile {
		return model.MediaFile{ID: id}
	})
}

// extractUserAndClient extracts user and client from the request context.
func extractUserAndClient(ctx context.Context) (model.User, string) {
	user, _ := request.UserFrom(ctx)
	client, _ := request.ClientFrom(ctx)
	return user, client
}

func getQueue(ds model.DataStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user, _ := request.UserFrom(ctx)
		repo := ds.PlayQueue(ctx)
		pq, err := repo.RetrieveWithMediaFiles(user.ID)
		if err != nil && !errors.Is(err, model.ErrNotFound) {
			log.Error(ctx, "Error retrieving queue", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if pq == nil {
			pq = &model.PlayQueue{}
		}
		resp, err := json.Marshal(pq)
		if err != nil {
			log.Error(ctx, "Error marshalling queue", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(resp) //nolint:gosec
	}
}

func saveQueue(ds model.DataStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		payload, ok := decodeUpdatePayload(w, r)
		if !ok {
			return
		}
		user, client := extractUserAndClient(ctx)
		ids := V(payload.Ids)
		items := createMediaFileItems(ids)
		current := V(payload.Current)
		if len(ids) > 0 && !validateCurrentIndex(w, current, len(ids)) {
			return
		}
		pq := &model.PlayQueue{
			UserID:    user.ID,
			Current:   current,
			Position:  max(V(payload.Position), 0),
			ChangedBy: client,
			Items:     items,
		}
		if err := ds.PlayQueue(ctx).Store(pq); err != nil {
			log.Error(ctx, "Error saving queue", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func updateQueue(ds model.DataStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Decode and validate the JSON payload
		payload, ok := decodeUpdatePayload(w, r)
		if !ok {
			return
		}

		// Extract user and client information from request context
		user, client := extractUserAndClient(ctx)

		// Initialize play queue with user ID and client info
		pq := &model.PlayQueue{UserID: user.ID, ChangedBy: client}
		var cols []string // Track which columns to update in the database

		// Handle queue items update
		if payload.Ids != nil {
			pq.Items = createMediaFileItems(*payload.Ids)
			cols = append(cols, "items")

			// If current index is not being updated, validate existing current index
			// against the new items list to ensure it remains valid
			if payload.Current == nil {
				existing, ok := retrieveExistingQueue(ctx, w, ds, user.ID)
				if !ok {
					return
				}
				if existing != nil && !validateCurrentIndex(w, existing.Current, len(*payload.Ids)) {
					return
				}
			}
		}

		// Handle current track index update
		if payload.Current != nil {
			pq.Current = *payload.Current
			cols = append(cols, "current")

			if payload.Ids != nil {
				// If items are also being updated, validate current index against new items
				if !validateCurrentIndex(w, *payload.Current, len(*payload.Ids)) {
					return
				}
			} else {
				// If only current index is being updated, validate against existing items
				existing, ok := retrieveExistingQueue(ctx, w, ds, user.ID)
				if !ok {
					return
				}
				if existing != nil && !validateCurrentIndex(w, *payload.Current, len(existing.Items)) {
					return
				}
			}
		}

		// Handle playback position update
		if payload.Position != nil {
			pq.Position = max(*payload.Position, 0) // Ensure position is non-negative
			cols = append(cols, "position")
		}

		// If no fields were specified for update, return success without doing anything
		if len(cols) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Perform partial update of the specified columns only
		if err := ds.PlayQueue(ctx).Store(pq, cols...); err != nil {
			log.Error(ctx, "Error updating queue", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func clearQueue(ds model.DataStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user, _ := request.UserFrom(ctx)
		if err := ds.PlayQueue(ctx).Clear(user.ID); err != nil {
			log.Error(ctx, "Error clearing queue", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func autofillQueue(ds model.DataStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user, _ := request.UserFrom(ctx)

		count := 1
		payload := autofillQueuePayload{}
		if body, err := io.ReadAll(r.Body); err == nil && len(bytes.TrimSpace(body)) > 0 {
			if err := json.Unmarshal(body, &payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if payload.Count > 0 {
			count = payload.Count
		} else if raw := r.URL.Query().Get("count"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 {
				http.Error(w, "count must be a positive integer", http.StatusBadRequest)
				return
			}
			count = n
		}

		queue, err := ds.PlayQueue(ctx).RetrieveWithMediaFiles(user.ID)
		if err != nil && !errors.Is(err, model.ErrNotFound) {
			log.Error(ctx, "Error retrieving queue for autofill", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if queue == nil {
			queue = &model.PlayQueue{UserID: user.ID}
		}

		pool, err := ds.MediaFile(ctx).GetAll(model.QueryOptions{Filters: squirrel.Eq{"missing": false}})
		if err != nil {
			log.Error(ctx, "Error loading media files for queue autofill", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(pool) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		existing := map[string]struct{}{}
		for _, item := range queue.Items {
			existing[item.ID] = struct{}{}
		}
		for _, id := range payload.ExcludeIDs {
			existing[id] = struct{}{}
		}
		candidates := make(model.MediaFiles, 0, len(pool))
		for _, item := range pool {
			if _, ok := existing[item.ID]; ok {
				continue
			}
			candidates = append(candidates, item)
		}
		if len(candidates) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		contextTracks := queue.Items
		if len(contextTracks) > 6 {
			contextTracks = contextTracks[len(contextTracks)-6:]
		}

		recommended := playlistsvc.NewTrackAffinity().Recommend(candidates, contextTracks, count)
		if len(recommended) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		queue.Items = append(queue.Items, recommended...)
		if queue.UserID == "" {
			queue.UserID = user.ID
		}
		if err := ds.PlayQueue(ctx).Store(queue, "items"); err != nil {
			log.Error(ctx, "Error autofilling queue", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(recommended)
	}
}

func saveQueueAsPlaylist(ds model.DataStore, pls playlistsvc.Playlists) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user, _ := request.UserFrom(ctx)

		var payload saveQueueAsPlaylistPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		queue, err := ds.PlayQueue(ctx).RetrieveWithMediaFiles(user.ID)
		if err != nil && !errors.Is(err, model.ErrNotFound) {
			log.Error(ctx, "Error retrieving queue for save-as-playlist", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if queue == nil || len(queue.Items) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		name := strings.TrimSpace(payload.Name)
		if name == "" {
			name = "Dynamic Mix"
		}

		ids := slice.Map(queue.Items, func(mf model.MediaFile) string { return mf.ID })
		playlistID, err := pls.Create(ctx, "", name, ids)
		if err != nil {
			log.Error(ctx, "Error saving queue as playlist", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}{ID: playlistID, Name: name})
	}
}
