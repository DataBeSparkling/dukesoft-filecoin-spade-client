package boostclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"filecoin-spade-client/pkg/config"
	"filecoin-spade-client/pkg/log"
	"filecoin-spade-client/pkg/spadeclient"
	"fmt"
	boostapi "github.com/filecoin-project/boost/api"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"io"
	"net/http"
	"time"
)

type BoostClient struct {
	Config        config.BoostConfig
	Api           boostapi.BoostStruct
	HttpTransport http.RoundTripper

	MinerAddress  address.Address
	WorkerAddress address.Address
}

func New(config config.Configuration) *BoostClient {
	bc := new(BoostClient)
	bc.Config = config.BoostConfig
	bc.HttpTransport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: config.InsecureSkipVerify},
	}

	return bc
}

func (bc *BoostClient) Start(ctx context.Context) {
	daemonCtx, cancelDaemon := context.WithCancel(ctx)
	bc.connectBoostDaemon(daemonCtx)

	go func() {
		select {
		case <-ctx.Done():
			log.Infof("shutting down boost client: context done")
			cancelDaemon()
			return
		}
	}()
}

func (bc *BoostClient) connectBoostDaemon(ctx context.Context) {
	// Setup daemon API
	closer, err := jsonrpc.NewMergeClient(
		context.Background(),
		bc.Config.BoostUrl,
		"Filecoin",
		[]interface{}{&bc.Api.Internal, &bc.Api.CommonStruct.Internal},
		http.Header{"Authorization": []string{"Bearer " + bc.Config.BoostAuthToken}},
	)
	if err != nil {
		log.Fatalf("connecting with Boost failed: %s", err)
	}

	// Check a simple call
	_, err = bc.Api.MarketGetAsk(ctx)
	if err != nil {
		log.Fatalf("failure checking Boost connection: %s", err)
	}

	// Also check graphQL
	_, err = bc.basicQuery(ctx, "deals(limit: 1) {totalCount}")
	if err != nil {
		log.Fatalf("failure checking GraphQL connection: %s", err)
	}

	log.Infof("Successfully connected to Boost.")

	go func() {
		select {
		case <-ctx.Done():
			log.Infof("shutting down boost client: context done")
			closer()
			return
		}
	}()
}

type GraphQLRequest struct {
	OperationName string      `json:"operationName,omitempty"`
	Query         string      `json:"query"`
	Variables     interface{} `json:"variables,omitempty"`
}

func (bc *BoostClient) basicQuery(ctx context.Context, query string) ([]byte, error) {
	return bc.graphQlQuery(ctx, GraphQLRequest{
		Query: fmt.Sprintf("query {%s}", query),
	})
}

func (bc *BoostClient) graphQlQuery(ctx context.Context, request GraphQLRequest) ([]byte, error) {
	requestJson, err := json.Marshal(request)
	if err != nil {
		return nil, xerrors.Errorf("could not serialize graphql request: %+s", err)
	}
	url := bc.Config.GraphQlUrl + "/graphql/query"

	req, _ := http.NewRequest("POST", url, bytes.NewReader(requestJson))
	req.Header.Set("Content-Type", "application/json")

	resp, err := bc.HttpTransport.RoundTrip(req)
	if err != nil {
		return []byte{}, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, xerrors.New(fmt.Sprintf("could not read graphql response: %s", err))
	}

	if resp.StatusCode != 200 {
		log.Debugf("graphql returned response body: %s", body)
		return []byte{}, xerrors.New(fmt.Sprintf("graphql returned %d instead of expected 200", resp.StatusCode))
	}

	return body, nil
}

type BoostDealsResponse struct {
	Data struct {
		Deals struct {
			Deals []struct {
				ID         uuid.UUID `json:"ID"`
				CreatedAt  time.Time `json:"CreatedAt"`
				Checkpoint string    `json:"Checkpoint"`
				IsOffline  bool      `json:"IsOffline"`
				Err        string    `json:"Err"`
				PieceCid   string    `json:"PieceCid"`
				Message    string    `json:"Message"`
			} `json:"deals"`
			TotalCount int `json:"totalCount"`
		} `json:"deals"`
	} `json:"data"`
}

func (bc *BoostClient) GetBoostDeals(ctx context.Context) (*BoostDealsResponse, error) {
	resp, err := bc.basicQuery(ctx, "deals(limit: 1000, filter: {IsOffline: true, Checkpoint: Accepted}) {deals {ID CreatedAt Checkpoint IsOffline Err PieceCid Message}totalCount}")
	if err != nil {
		return nil, err
	}

	var responseObject BoostDealsResponse
	err = json.Unmarshal(resp, &responseObject)
	if err != nil {
		log.Warnf("Could not unmarshal data:\n%s", resp)
		return nil, err
	}

	return &responseObject, nil
}

func (bc *BoostClient) ImportDeal(ctx context.Context, proposal *spadeclient.DealProposal, filepath string) error {
	log.Infof("Importing Boost deal %s with data %s", proposal.ProposalID, filepath)
	actualUuid, err := uuid.Parse(proposal.ProposalID)
	if err != nil {
		return err
	}
	response, err := bc.Api.BoostOfflineDealWithData(ctx, actualUuid, filepath, true)
	if err != nil {
		return err
	}
	if response.Accepted != true {
		return xerrors.Errorf("Importer did not accept deal: %s", response.Reason)
	}
	return nil
}

type BoostCancelDealResponse struct {
	Data struct {
		DealCancel string `json:"dealCancel"`
	} `json:"data"`
}

func (bc *BoostClient) CancelDeal(ctx context.Context, dealId string) error {
	vars := struct {
		Id string `json:"id"`
	}{
		Id: dealId,
	}
	request := GraphQLRequest{
		OperationName: "AppDealCancelMutation",
		Query:         "mutation AppDealCancelMutation($id: ID!) { dealCancel(id: $id) }",
		Variables:     vars,
	}

	//log.Infof("Actual request: %+v", request)

	resp, err := bc.graphQlQuery(ctx, request)
	if err != nil {
		return err
	}

	var responseObject BoostCancelDealResponse
	err = json.Unmarshal(resp, &responseObject)
	if err != nil {
		log.Warnf("Could not unmarshal data:\n%s", resp)
		return err
	}

	if responseObject.Data.DealCancel != dealId {
		return xerrors.Errorf("Did not properly cancel deal %s: response had %s", dealId, responseObject.Data.DealCancel)
	}

	return nil
}
