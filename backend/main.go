package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Data Model

type ParticipantType string

const (
	Human ParticipantType = "human"
	Agent ParticipantType = "agent"
)

type Participant struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Type         ParticipantType `json:"type"`
	RepresentsID string          `json:"represents_id,omitempty"` // if Type==Agent, the human it acts for
}

type Option struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	ProposedBy string `json:"proposed_by"`
}

type Argument struct {
	ID            string `json:"id"`
	OptionID      string `json:"option_id"`
	ParticipantID string `json:"participant_id"`
	Text          string `json:"text"`
}

type Vote struct {
	ParticipantID string    `json:"participant_id"`
	OptionID      string    `json:"option_id"`
	Overridden    bool      `json:"overridden"` // true if this vote replaced their own agent's vote
	Timestamp     time.Time `json:"timestamp"`
}

type Board struct {
	Topic        string        `json:"topic"`
	Status       string        `json:"status"` // "open" | "closed"
	Participants []Participant `json:"participants"`
	Options      []Option      `json:"options"`
	Arguments    []Argument    `json:"arguments"`
	// Votes keyed by participant ID -- each participant has exactly one active vote
	Votes map[string]Vote `json:"votes"`
}

// ---------- In-memory store ----------

var (
	mu    sync.Mutex
	board Board
	idSeq int
)

func nextID() string {
	idSeq++
	return time.Now().Format("150405") + "-" + strconv.Itoa(idSeq)
}

func seedBoard() {
	board = Board{
		Topic:  "Where should the team hold its Q4 offsite?",
		Status: "open",
		Participants: []Participant{
			{ID: "p-thompson", Name: "Thompson", Type: Human},
			{ID: "p-thompson-agent", Name: "Thompson's Agent", Type: Agent, RepresentsID: "p-thompson"},
			{ID: "p-ada", Name: "Ada", Type: Human},
			{ID: "p-ada-agent", Name: "Ada's Agent", Type: Agent, RepresentsID: "p-ada"},
			{ID: "p-musa-agent", Name: "Musa's Agent", Type: Agent, RepresentsID: "p-musa"},
		},
		Options:   []Option{},
		Arguments: []Argument{},
		Votes:     map[string]Vote{},
	}
}

// ---------- Helpers ----------

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// ---------- Handlers ----------

// GET /api/board -- full current board state
func handleGetBoard(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	mu.Lock()
	defer mu.Unlock()
	writeJSON(w, board)
}

// POST /api/options {text, participant_id}
func handleProposeOption(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == http.MethodOptions {
		return
	}
	var req struct {
		Text          string `json:"text"`
		ParticipantID string `json:"participant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	opt := Option{ID: nextID(), Text: req.Text, ProposedBy: req.ParticipantID}
	board.Options = append(board.Options, opt)
	writeJSON(w, opt)
}

// POST /api/arguments {option_id, participant_id, text}
func handleArgueFor(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == http.MethodOptions {
		return
	}
	var req struct {
		OptionID      string `json:"option_id"`
		ParticipantID string `json:"participant_id"`
		Text          string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	arg := Argument{ID: nextID(), OptionID: req.OptionID, ParticipantID: req.ParticipantID, Text: req.Text}
	board.Arguments = append(board.Arguments, arg)
	writeJSON(w, arg)
}

// POST /api/vote {option_id, participant_id}
// If participant_id is a human who has an agent, and that agent's current
// vote differs from this one, this vote is marked as an override.
func handleCastVote(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == http.MethodOptions {
		return
	}
	var req struct {
		OptionID      string `json:"option_id"`
		ParticipantID string `json:"participant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mu.Lock()
	defer mu.Unlock()

	overridden := false
	// find this participant's record
	var actor *Participant
	for i := range board.Participants {
		if board.Participants[i].ID == req.ParticipantID {
			actor = &board.Participants[i]
			break
		}
	}
	if actor != nil && actor.Type == Human {
		// does this human have an agent, and did it vote differently?
		for _, p := range board.Participants {
			if p.Type == Agent && p.RepresentsID == actor.ID {
				if av, ok := board.Votes[p.ID]; ok && av.OptionID != req.OptionID {
					overridden = true
				}
			}
		}
	}

	board.Votes[req.ParticipantID] = Vote{
		ParticipantID: req.ParticipantID,
		OptionID:      req.OptionID,
		Overridden:    overridden,
		Timestamp:     time.Now(),
	}
	writeJSON(w, board.Votes[req.ParticipantID])
}

func main() {
	seedBoard()

	http.HandleFunc("/api/board", handleGetBoard)
	http.HandleFunc("/api/options", handleProposeOption)
	http.HandleFunc("/api/arguments", handleArgueFor)
	http.HandleFunc("/api/vote", handleCastVote)
	http.Handle("/", http.FileServer(http.Dir("./static")))

	log.Println("Deliberation board listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
