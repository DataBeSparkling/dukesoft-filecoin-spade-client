package client

import (
	"context"
	"filecoin-spade-client/pkg/ariaclient"
	"filecoin-spade-client/pkg/boostclient"
	"filecoin-spade-client/pkg/config"
	"filecoin-spade-client/pkg/log"
	"filecoin-spade-client/pkg/lotusclient"
	"filecoin-spade-client/pkg/spadeclient"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Client struct {
	Configuration           config.Configuration
	LotusClient             *lotusclient.LotusClient
	SpadeClient             *spadeclient.SpadeClient
	BoostClient             *boostclient.BoostClient
	AriaClient              *ariaclient.AriaClient
	DuplicateDeals          map[string]string
	DuplicateDealsMutex     sync.Mutex
	ActiveDeals             map[string]*spadeclient.DealProposal
	ActiveDealsMutex        sync.Mutex
	ImportedDeals           map[string]bool
	ImportedDealsMutex      sync.Mutex
	WaitingForProposal      map[string]bool
	WaitingForProposalMutex sync.Mutex
	FailureMap              sync.Map
}

func New(config config.Configuration) *Client {
	cl := new(Client)
	cl.Configuration = config
	cl.LotusClient = lotusclient.New(config)
	cl.SpadeClient = spadeclient.New(config, cl.LotusClient)
	cl.BoostClient = boostclient.New(config)
	cl.AriaClient = ariaclient.New(config)
	cl.DuplicateDeals = make(map[string]string)
	cl.ActiveDeals = make(map[string]*spadeclient.DealProposal)
	cl.ImportedDeals = make(map[string]bool)
	cl.WaitingForProposal = make(map[string]bool)
	return cl
}

func (cl *Client) Start(ctx context.Context) error {
	log.Infof("Starting Spade Client...")
	ariactx, cancelAria := context.WithCancel(ctx)
	defer cancelAria()
	cl.AriaClient.Start(ariactx)

	newctx, cancelClient := context.WithCancel(ctx)
	defer cancelClient()
	cl.LotusClient.Start(newctx)

	boostctx, cancelBoost := context.WithCancel(ctx)
	defer cancelBoost()
	cl.BoostClient.Start(boostctx)

	spadectx, cancelSpade := context.WithCancel(ctx)
	defer cancelSpade()
	cl.SpadeClient.Start(spadectx)

	//log.Infof("Requesting sealing pipeline")
	//sealing, err := cl.BoostClient.GetBoostSealingPipeline(ctx)
	//log.Infof("Spade deal data: %+v (%+v)", sealing, err)

	log.Infof("Spade client successfully started - starting main loop")
	go cl.scanPendingProposals(spadectx)

	select {
	case <-ctx.Done():
		log.Infof("shutting down spade client: context done")
		return nil
	}
}

func (cl *Client) scanPendingProposals(ctx context.Context) {
	log.Infof("Scanning pending proposals with a ticker interval of %s", cl.SpadeClient.Config.PendingRefreshInterval.String())
	ticker := time.NewTicker(cl.SpadeClient.Config.PendingRefreshInterval)
	defer ticker.Stop()

	totalRequested := 0

	for {
		var boostDeals *boostclient.BoostDealsResponse

		log.Infof("> Fetching pending proposals")
		pendingProposals, err := cl.SpadeClient.PendingProposals(ctx)
		if err != nil {
			log.Warnf(" > Could not fetch pending proposals: %+s", err)
			goto ticker
		}

		log.Infof(" > %d pending proposals, %d recent failures", len(pendingProposals.PendingProposals), len(pendingProposals.RecentFailures))

		// We take these failures, and if they are indeed duplicate failures, we cancel them
		for _, failure := range pendingProposals.RecentFailures {
			if strings.Index(failure.Error, "deal proposal is identical to deal") != -1 {
				r, _ := regexp.Compile(`[a-z0-9]{8}-[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{12}`)
				duplicate := r.FindString(failure.Error)
				if duplicate != "" {
					if cl.HasDuplicateDeal(failure.PieceCid) {
						continue
					}

					log.Warnf("  > Captured a duplicate deal - cancelling deal in Boost")
					log.Warnf("   > PieceCID: %s", failure.PieceCid)
					log.Warnf("   > Proposal: %s", failure.ProposalID)
					log.Warnf("   > Duplicate of: %s", duplicate)
					log.Warnf("    > Cancelling Boost deal %s", duplicate)

					go func() {
						_ = cl.BoostClient.CancelDeal(ctx, duplicate) // we dont care about the outcome and its slow
					}()

					cl.AddDuplicateDeal(failure.PieceCid, duplicate)

					// Also add it so the spade client, so we don't re-request it
					// in hindsight, lets not do that, and re-request it after we've cancelled the original one :)
					// in hindsight again, lets do do that, because spade doesn't care that we've deleted something
					// and it still shows the errors
					cl.SpadeClient.AddRequestedPiece(failure.PieceCid)

					// Also remove it from our requested pieces waiting list so we make some space for other deals
					cl.RemoveWaitingForProposal(failure.PieceCid)

					// @TODO -> Remove the duplicate entry from the requested pieces and whatnot, after it expired
					//   then we can request it again
				}
			} else {
				if strings.Index(failure.Error, "cannot seal a sector before") != -1 {
					// we can not re-request this deal, the requested expiration will not be changed on a new request
					cl.SpadeClient.AddRequestedPiece(failure.PieceCid)
				}

				_, ok := cl.FailureMap.Load(failure.PieceCid)
				if ok == false {
					cl.RemoveWaitingForProposal(failure.PieceCid)
					if strings.Index(failure.Error, "PHP Fatal error") == -1 {
						cl.FailureMap.Store(failure.PieceCid, failure.Error)
						log.Warnf("   > PieceCID %s failed with %s", failure.PieceCid, failure.Error)
					} else {
						log.Warnf("   > PieceCID %s failed with %s local failure, not adding to failure map", failure.PieceCid, failure.Error)
					}
				}
			}
		}

		//log.Debugf("PENDING PROPOSALS: \n %+v", pendingProposals.PendingProposals)

		if len(pendingProposals.PendingProposals) == 0 {
			//no pending proposals, lets skip the deal checking in boost
			goto scanNewDeals
		}
		log.Infof("> fetching open deals from Boost...")

		boostDeals, err = cl.BoostClient.GetBoostDeals(ctx)
		if err != nil {
			log.Warnf(" > Could not fetch deals from boost: %+s", err)
			goto ticker
		}
		log.Infof(" > found %d deals in boost", boostDeals.Data.Deals.TotalCount)
		// try to match deals to pending proposals
	exit:
		for _, deal := range boostDeals.Data.Deals.Deals {
			// find it in the proposals
			for _, proposal := range pendingProposals.PendingProposals {
				if proposal.ProposalID == deal.ID.String() {
					//log.Infof("  > Matched deal %s (proposalID=%s) [PieceCID=%s]", deal.ID, proposal.ProposalID, deal.PieceCid)
					cl.RemoveWaitingForProposal(proposal.PieceCid)
					go cl.HandleDeal(ctx, proposal)
					continue exit
				} else {
					// this should never happen, bug in Spade (handled above here in the failures)

					//if proposal.PieceCid == deal.PieceCid {
					//	log.Warnf("  > MATCHED ON PIECE %s (proposalID=%s) [PieceCID=%s]", deal.ID, proposal.ProposalID, deal.PieceCid)
					//}
				}
			}

			//log.Infof(">>>> Did not find a proposal<->boost match for %s (%s)", deal.ID, deal.PieceCid)
		}

	scanNewDeals:
		// Now check if we should request some more proposals
		cl.ActiveDealsMutex.Lock()
		totalRequested = len(cl.ActiveDeals) + cl.GetAmountWaitingForProposal()
		if totalRequested < cl.Configuration.MaxSpadeDealsActive {
			repeat := cl.Configuration.MaxSpadeDealsActive - totalRequested
			log.Infof("Currently handling %d deals (%d active, %d requested), less than given limit of %d, requesting new deal %d times", totalRequested, len(cl.ActiveDeals), cl.GetAmountWaitingForProposal(), cl.Configuration.MaxSpadeDealsActive, repeat)
			for i := 0; i < repeat; i++ {
				requested, err := cl.SpadeClient.RequestNewDeal(ctx)
				if err != nil {
					log.Warnf("Could not request new deal from Spade: %s", err)
					i = repeat // make sure we stop trying
				} else {
					cl.AddWaitingForProposal(requested)
				}
			}
		} else {
			log.Infof("Currently handling %d deals (%d active, %d requested), not requesting new deals", totalRequested, len(cl.ActiveDeals), cl.GetAmountWaitingForProposal())
		}
		cl.ActiveDealsMutex.Unlock()
	ticker:
		select {
		case <-ticker.C: // Return back into the loop
		case <-ctx.Done():
			log.Infof("Stopping pending proposal worker: context done")
			return
		}
	}
}

func (cl *Client) HasDuplicateDeal(pieceCid string) bool {
	return cl.GetDuplicateDeal(pieceCid) != ""
}

func (cl *Client) GetDuplicateDeal(pieceCid string) string {
	cl.DuplicateDealsMutex.Lock()
	defer cl.DuplicateDealsMutex.Unlock()

	if _, ok := cl.DuplicateDeals[pieceCid]; ok {
		return cl.DuplicateDeals[pieceCid]
	}
	return ""
}

func (cl *Client) AddDuplicateDeal(pieceCid string, realProposalId string) {
	cl.DuplicateDealsMutex.Lock()
	defer cl.DuplicateDealsMutex.Unlock()

	cl.DuplicateDeals[pieceCid] = realProposalId
}

func (cl *Client) IsAlreadyImported(proposalID string) bool {
	cl.ImportedDealsMutex.Lock()
	defer cl.ImportedDealsMutex.Unlock()

	if _, ok := cl.ImportedDeals[proposalID]; ok {
		return true
	}
	return false
}

func (cl *Client) AddImported(proposalID string) {
	cl.ImportedDealsMutex.Lock()
	defer cl.ImportedDealsMutex.Unlock()

	cl.ImportedDeals[proposalID] = true
}

func (cl *Client) GetAmountWaitingForProposal() int {
	cl.WaitingForProposalMutex.Lock()
	defer cl.WaitingForProposalMutex.Unlock()

	return len(cl.WaitingForProposal)
}

func (cl *Client) AddWaitingForProposal(proposalID string) {
	cl.WaitingForProposalMutex.Lock()
	defer cl.WaitingForProposalMutex.Unlock()

	cl.WaitingForProposal[proposalID] = true
}

func (cl *Client) RemoveWaitingForProposal(proposalID string) {
	cl.WaitingForProposalMutex.Lock()
	defer cl.WaitingForProposalMutex.Unlock()

	delete(cl.WaitingForProposal, proposalID)
}

func (cl *Client) HandleDeal(ctx context.Context, proposal spadeclient.DealProposal) {
	// check if we're already handling this deal
	cl.ActiveDealsMutex.Lock()
	if _, ok := cl.ActiveDeals[proposal.ProposalID]; ok {
		cl.ActiveDealsMutex.Unlock()
		//log.Debugf("Already handling deal %s", proposal.ProposalID)
		return
	}

	// check if it maybe already was imported
	if cl.IsAlreadyImported(proposal.ProposalID) {
		cl.ActiveDealsMutex.Unlock()
		//log.Debugf("Already imported deal %s", proposal.ProposalID)
		return
	}

	// Now check if we're not doing too many deals
	if len(cl.ActiveDeals) > cl.Configuration.MaxSpadeDealsActive {
		cl.ActiveDealsMutex.Unlock()
		//log.Debugf("Already handling %d deals - backing off from handling %s", len(cl.ActiveDeals), proposal.ProposalID)
		return
	}

	cl.ActiveDeals[proposal.ProposalID] = &proposal
	cl.ActiveDealsMutex.Unlock()

	retry := 0
handleDeal:
	if retry > 10 {
		log.Errorf("Could not handle deal %s! Giving up.", proposal.ProposalID)
		cl.ActiveDealsMutex.Lock()
		delete(cl.ActiveDeals, proposal.ProposalID)
		cl.ActiveDealsMutex.Unlock()
		return
	}

	log.Infof("Fetching manifest for %s", proposal.ProposalID)
	manifest, err := cl.SpadeClient.RequestPieceManifest(ctx, proposal.ProposalID)
	if err != nil {
		log.Warnf(" > Could not fetch manifest: %+s", err)
		time.Sleep(time.Second)
		retry++
		goto handleDeal
	}

	outFilename := fmt.Sprintf("%s/%s", cl.Configuration.DownloadPath, manifest.FRC58CommP.PCidV2())
	log.Debugf("Found %d segments, starting download and assembly (%s)", len(manifest.PieceList), outFilename)

	// Check if we already have an active download for this source
	err = manifest.StartDownload(ctx, outFilename, true, 50, 60*10, false, 5)
	if err != nil {
		log.Infof("Download errored %s (%s) - stopping and removing", proposal.ProposalID, err.Error())

		// remove from actual list
		cl.ActiveDealsMutex.Lock()
		delete(cl.ActiveDeals, proposal.ProposalID)
		cl.ActiveDealsMutex.Unlock()
		return
	}
	log.Infof(" > Download handler done for %s", proposal.ProposalID)

	err = cl.BoostClient.ImportDeal(ctx, &proposal, outFilename)
	if err != nil {
		log.Warnf("Failure importing boost deal %s: %s", proposal.ProposalID, err)
		return
	}

	// remove from actual list
	cl.ActiveDealsMutex.Lock()
	delete(cl.ActiveDeals, proposal.ProposalID)
	cl.ActiveDealsMutex.Unlock()

	// also add to imported list
	cl.AddImported(proposal.ProposalID)

	log.Infof("Successfully downloaded and imported %s", proposal.ProposalID)
	return
}

// startDownloadOrAttachToExisting returns a GID as a string that can be used to check status
//func (cl *Client) startDownloadOrAttachToExisting(proposal *spadeclient.DealProposal) (string, error) {
//	for _, dealSource := range proposal.Sources {
//		//log.Infof("Finding URL [%s]", dealSource)
//		foundDownload := cl.AriaClient.FindDownloadByUri(dealSource)
//		if foundDownload != nil {
//			log.Infof("   > Found existing download %s: %s", foundDownload.GID, foundDownload.Status)
//			return foundDownload.GID, nil
//		}
//	}
//
//	//log.Infof("   > No existing download found - creating new")
//	if len(proposal.Sources) > 1 {
//		log.Warnf("Multiple sources found for %s! Only using first source.", proposal.ProposalID)
//	}
//	if len(proposal.Sources) == 0 {
//		log.Warnf("No sources found for %s: %+v", proposal.ProposalID, proposal)
//	}
//	status, err := cl.AriaClient.NewDownload(proposal.Sources[0], path.Base(proposal.Sources[0]))
//	if err != nil {
//		return "", err
//	}
//	return status.GID, nil
//}

//func (cl *Client) monitorDownload(ctx context.Context, gid string, proposal *spadeclient.DealProposal) {
//	log.Infof("Monitoring download %s", gid)
//
//	log.Infof("Fetching manifest %s", gid)
//
//	// @todo make use of the Aria2C pub/sub events to make this quicker
//	ticker := time.NewTicker(time.Second * 10)
//	defer ticker.Stop()
//
//	for {
//		status, err := cl.AriaClient.GetStatus(gid)
//		if err != nil {
//			log.Warnf("Error getting status for download %s: %s", gid, err)
//		} else {
//			if status.Status == "error" {
//				log.Infof("Download errored %s (%s) - stopping and removing", status.GID, proposal.ProposalID)
//				// Remove from aria2c
//				err = cl.AriaClient.RemoveDownload(status.GID)
//				if err != nil {
//					log.Warnf("Could not remove download from Aria2C: %s", err)
//				}
//				//log.Infof("Removed %s (%s) from aria2c", status.GID, proposal.ProposalID)
//
//				// remove from actual list
//				cl.ActiveDealsMutex.Lock()
//				delete(cl.ActiveDeals, proposal.ProposalID)
//				cl.ActiveDealsMutex.Unlock()
//				return
//			}
//
//			if status.Status == "complete" {
//				log.Infof("Download finished %s (%s)", status.GID, proposal.ProposalID)
//				err := cl.BoostClient.ImportDeal(ctx, proposal, status.Files[0].Path)
//				if err != nil {
//					log.Warnf("Failure importing boost deal %s: %s", proposal.ProposalID, err)
//					return
//				}
//				//log.Infof("Handled deal %s (%s) - removing from Aria2c", status.GID, proposal.ProposalID)
//
//				// Remove from aria2c
//				err = cl.AriaClient.RemoveDownload(status.GID)
//				if err != nil {
//					log.Warnf("Could not remove download from Aria2C: %s", err)
//				}
//				//log.Infof("Removed %s (%s) from aria2c", status.GID, proposal.ProposalID)
//
//				// remove from actual list
//				cl.ActiveDealsMutex.Lock()
//				delete(cl.ActiveDeals, proposal.ProposalID)
//				cl.ActiveDealsMutex.Unlock()
//
//				// also add to imported list
//				cl.AddImported(proposal.ProposalID)
//
//				log.Infof("Successfully downloaded and imported %s", proposal.ProposalID)
//				return
//			}
//
//			percentage := 0.0
//			if status.CompletedLength != 0 && status.TotalLength != 0 {
//				percentage = (float64(status.CompletedLength) / float64(status.TotalLength)) * 100.0
//			}
//			log.Infof("Download %s (%s): %s @ %s/s [%s / %s (%3.2f%%)]", status.GID, proposal.ProposalID, status.Status, humanize.Bytes(uint64(status.DownloadSpeed)), humanize.Bytes(uint64(status.CompletedLength)), humanize.Bytes(uint64(status.TotalLength)), percentage)
//		}
//
//		select {
//		case <-ticker.C: // Return back into the loop
//		case <-ctx.Done():
//			log.Infof("Stopping download monitoring %s: context done", gid)
//			return
//		}
//	}
//}
