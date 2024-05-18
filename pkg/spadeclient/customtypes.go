package spadeclient

import (
	apitypes "github.com/data-preservation-programs/go-spade-apitypes"
	fildatasegment "github.com/ribasushi/fil-datasegment/pkg/dlass"
	"time"
)

/// These are custom types because the apitypes isn't reflective of the actual API

type PendingProposalResponseEnvelope struct { // Copied from apitypes.ResponseEnvelope
	RequestID          string                   `json:"request_id,omitempty"`
	ResponseTime       time.Time                `json:"response_timestamp"`
	ResponseStateEpoch int64                    `json:"response_state_epoch,omitempty"`
	ResponseCode       int                      `json:"response_code"`
	ErrCode            int                      `json:"error_code,omitempty"`
	ErrSlug            string                   `json:"error_slug,omitempty"`
	ErrLines           []string                 `json:"error_lines,omitempty"`
	InfoLines          []string                 `json:"info_lines,omitempty"`
	ResponseEntries    *int                     `json:"response_entries,omitempty"`
	Response           ResponsePendingProposals `json:"response"`
}

type EligiblePiecesResponseEnvelope struct { // Copied from apitypes.ResponseEnvelope
	RequestID          string    `json:"request_id,omitempty"`
	ResponseTime       time.Time `json:"response_timestamp"`
	ResponseStateEpoch int64     `json:"response_state_epoch,omitempty"`
	ResponseCode       int       `json:"response_code"`
	ErrCode            int       `json:"error_code,omitempty"`
	ErrSlug            string    `json:"error_slug,omitempty"`
	ErrLines           []string  `json:"error_lines,omitempty"`
	InfoLines          []string  `json:"info_lines,omitempty"`
	ResponseEntries    *int      `json:"response_entries,omitempty"`
	Response           []*Piece  `json:"response"`
}

type Piece struct { //copied from apitypes.Piece (didn't incluyde policy ID)
	PieceCid         string   `json:"piece_cid"`
	PaddedPieceSize  uint64   `json:"padded_piece_size"`
	ClaimingTenants  []int16  `json:"tenants" db:"tenant_ids"`
	TenantPolicyCid  string   `json:"tenant_policy_cid"`
	SampleRequestCmd string   `json:"sample_request_cmd"`
	Sources          []string `json:"sources,omitempty"`
}

type ResponsePendingProposals struct { // copied from apitypes.ResponsePendingProposals
	RecentFailures   []apitypes.ProposalFailure `json:"recent_failures,omitempty"`
	PendingProposals []DealProposal             `json:"pending_proposals"`
}

type ResponseInvokeEnvelope struct { // Copied from apitypes.ResponseEnvelope
	RequestID          string         `json:"request_id,omitempty"`
	ResponseTime       time.Time      `json:"response_timestamp"`
	ResponseStateEpoch int64          `json:"response_state_epoch,omitempty"`
	ResponseCode       int            `json:"response_code"`
	ErrCode            int            `json:"error_code,omitempty"`
	ErrSlug            string         `json:"error_slug,omitempty"`
	ErrLines           []string       `json:"error_lines,omitempty"`
	InfoLines          []string       `json:"info_lines,omitempty"`
	ResponseEntries    *int           `json:"response_entries,omitempty"`
	Response           ResponseInvoke `json:"response"`
}

type ResponsePieceManifestEnvelope struct { // Copied from apitypes.ResponseEnvelope
	RequestID          string             `json:"request_id,omitempty"`
	ResponseTime       time.Time          `json:"response_timestamp"`
	ResponseStateEpoch int64              `json:"response_state_epoch,omitempty"`
	ResponseCode       int                `json:"response_code"`
	ErrCode            int                `json:"error_code,omitempty"`
	ErrSlug            string             `json:"error_slug,omitempty"`
	ErrLines           []string           `json:"error_lines,omitempty"`
	InfoLines          []string           `json:"info_lines,omitempty"`
	Response           fildatasegment.Agg `json:"response"`
}

type ResponseInvoke struct {
}

type DealProposal struct { // copied from apitypes.DealProposal
	ProposalID     string    `json:"deal_proposal_id"`
	ProposalCid    *string   `json:"deal_proposal_cid,omitempty"`
	HoursRemaining int       `json:"hours_remaining"`
	PieceSize      int64     `json:"piece_size"`
	PieceCid       string    `json:"piece_cid"`
	TenantID       int16     `json:"tenant_id"`
	TenantClient   string    `json:"tenant_client_id"`
	StartTime      time.Time `json:"deal_start_time"`
	StartEpoch     int64     `json:"deal_start_epoch"`
	ImportCmd      string    `json:"sample_import_cmd"`
	//Sources        []string  `json:"data_sources,omitempty"` // this JSON entry differs
}
