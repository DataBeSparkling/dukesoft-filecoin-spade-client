package client

import (
	"context"
	"filecoin-spade-client/pkg/ariaclient"
	"filecoin-spade-client/pkg/boostclient"
	"filecoin-spade-client/pkg/config"
	"filecoin-spade-client/pkg/log"
	"filecoin-spade-client/pkg/lotusclient"
	"filecoin-spade-client/pkg/spadeclient"
	"github.com/dustin/go-humanize"
	"golang.org/x/exp/slices"
	"path"
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

	tmpIgnoreDuplicateDealReports := []string{
		"baga6ea4seaqjjubu2zd33w7yvirxeymt6civduuctt5vhuvzinmcslekarkywga",
		"baga6ea4seaqd7rydprfubxkklht533xjpy7aql2gwtn5x6kouh57o5pllrebcly",
		"baga6ea4seaqm3zlbiexmjl2rd5qrnmwldh24twyprxtyzs36d36zvgwkhnmvyii",
		"baga6ea4seaqlsvlqh5ud24uh7vowc3lopoffix55t6mmot2jgm5dxwdbrhjneii",
		"baga6ea4seaqka5bzeqysuvh5yvhwpfgwjwzrwuql3p6tiqm3g7u7xvt2ny5jcdi",
		"baga6ea4seaqkczl5ffhvpqirpts4a7whvjx6xghkspzfbntbynj4aooksukfamq",
		"baga6ea4seaqoz2gfuzz4aluguchqhdtku5kgfqwwdobrckdcbvqt2jc7ifvgclq",
		"baga6ea4seaqok7p6b5qww3uf67j65fqvnkxlce4hqt2xrxkgan4atkstnukemma",
		"baga6ea4seaqkngiqocx5zjks7x7rhufuwcw7vhscwi6j2zjtxkgvq4tdcekbqla",
		"baga6ea4seaqlhfmdw6lmicu47nnaha6ixhfpx2vvceojzrhhlrygipkq6wsruoi",
		"baga6ea4seaqn5lnyu6paoszmwebdbtkhq653qo6idcd3jofxmnpewkaloymfwkq",
		"baga6ea4seaqnl6x4tcbsrx4p5uwr2x6w2f44ezbu7vqpcjwh2z4m4jry4qyuway",
		"baga6ea4seaqnxyeaunvpr5i5kfl5fpnu25bduljvpyzsowthitvd3inabcwaibq",
		"baga6ea4seaqhefqeknqb4azir2rmr4k5fevit2mo5y6zzdearij5rpoyh7pt6ny",
		"baga6ea4seaqguhn5qv2vpvxho76yccplnf3ez5hs7clyfalh6wla2goy6zljyhq",
		"baga6ea4seaqpm7jc542mjgaxuap36dhf7yjvgeqypnjbsdahcq66dgdwfvlywjq",
		"baga6ea4seaqe3rdclkaaa3ognjkkxb3zkaftbm5hbdhaeq2huced7mdue5jccfi",
		"baga6ea4seaqpbp5lzmuhmpfifehshe2ogs2e7re6xus5iux74mc5pwy3l63a6na",
		"baga6ea4seaqn53daumrft6l2top6xmskjq273uo47plrkx6wzpguwftd435xojq",
		"baga6ea4seaqckmxul7gttxp2v7ylxll25ml77qnyikeimtwtxgzwb67gh4iogky",
		"baga6ea4seaqisv5yzvy3kevipy6acu3vcyazt2rrgptajpjtudkoumofof7oodq",
		"baga6ea4seaqfoyouwapqrnmg74q7djfnbtiv7hpout3hwp6ts2tzdhzmdifc6di",
		"baga6ea4seaqb56tmbenmawhbu3qaxy5khqccccovjphfoqlnsamfacv74fjwgla",
		"baga6ea4seaqlw3gyzpgdpo3fjbhgi3xuajkzokhmqcz3t767fqaqzjgxzy3i4mi",
		"baga6ea4seaqmdf5i2jn4fxc3xk6xmp72goyj3m4lfuauc2aon6h6jqrn5aepoeq",
		"baga6ea4seaqipjgepwteyol7zrjgrgmp3r65zmqqayxvecmu4fsg3sd3olygsoq",
		"baga6ea4seaqoq3bsuopxvlrlfffza4mduaggs2w7qeqycnraosizzckjwbdrwmq",
		"baga6ea4seaqmp2q4v7gydcn6ogasgnbfq6l645yeb3i2apehgjtombgvehjjwna",
		"baga6ea4seaqfge7xlfvyoefrrwj3m55dkie4ju53azlklll3yb3pmj5ybyoqmgi",
		"baga6ea4seaqkut42p3jobsmgarf4hzl2akh6cwl2nmkonqg6ilgxdaxztm7vmii",
		"baga6ea4seaqipbitn2bvt63hykgsnzfzcw4uvpbb5fneqku7apx6uf742p3ukfy",
		"baga6ea4seaqkwspzcxaa46bjjtyji4g35nmargxyely32k4pniyquhx4mup6iiy",
		"baga6ea4seaqealrqvjvixnjhmencpftlc4ghgfhdqhfkff4o4fpwbgcd5etm4li",
		"baga6ea4seaqmlk4u4ifvovh5r5dr2d7mya3yi4awrdjtgp7zt7xourdxzhkd4ay",
		"baga6ea4seaqhcv4xquopctzi7tga3tgoid5lugxne4youyicgg5tr7uuhxrmapa",
		"baga6ea4seaqlpkbi66u6ppgv3skh2vy2ecru2vhcfyafo4lohnlwjp3aiywaqiy",
		"baga6ea4seaqb4iszz73v5kqx7bb67oxus4ewfjcaz6bqlric4p4xe5k4qzbo2ci",
		"baga6ea4seaqhi4pekonht6lh3gvnlfcykzgrwqk3dhcglkdcptp6cp6dq7agqay",
	}

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
				if slices.Contains(tmpIgnoreDuplicateDealReports, failure.PieceCid) {
					continue // Temporarily SKIP these piece cids as they are manually removed from the DB and _can_ be requested
				}
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

	log.Infof("Started handling deal %s", proposal.ProposalID)
	if len(proposal.Sources) > 1 {
		log.Warnf("More than 1 source found for deal: %s (%+v)", proposal.ProposalID, proposal.Sources)
	}
	//for i, source := range proposal.Sources {
	//	log.Infof(" > Source %d: %s", i, source)
	//}

	// Check if we already have an active download for this source
	activeGid, err := cl.startDownloadOrAttachToExisting(&proposal)
	if err != nil {

		retry++
		duration := time.Duration(10*retry) * time.Second
		log.Warnf("Error starting download for %s: %s | sleeping for %s and retrying", proposal.ProposalID, err, duration)
		time.Sleep(duration)
		goto handleDeal
	}

	cl.monitorDownload(ctx, activeGid, &proposal)
	log.Infof(" > Download handler done for %s", proposal.ProposalID)
}

// startDownloadOrAttachToExisting returns a GID as a string that can be used to check status
func (cl *Client) startDownloadOrAttachToExisting(proposal *spadeclient.DealProposal) (string, error) {
	for _, dealSource := range proposal.Sources {
		//log.Infof("Finding URL [%s]", dealSource)
		foundDownload := cl.AriaClient.FindDownloadByUri(dealSource)
		if foundDownload != nil {
			log.Infof("   > Found existing download %s: %s", foundDownload.GID, foundDownload.Status)
			return foundDownload.GID, nil
		}
	}

	//log.Infof("   > No existing download found - creating new")
	if len(proposal.Sources) > 1 {
		log.Warnf("Multiple sources found for %s! Only using first source.", proposal.ProposalID)
	}
	if len(proposal.Sources) == 0 {
		log.Warnf("No sources found for %s: %+v", proposal.ProposalID, proposal)
	}
	status, err := cl.AriaClient.NewDownload(proposal.Sources[0], path.Base(proposal.Sources[0]))
	if err != nil {
		return "", err
	}
	return status.GID, nil
}

func (cl *Client) monitorDownload(ctx context.Context, gid string, proposal *spadeclient.DealProposal) {
	log.Infof("Monitoring download %s", gid)
	// @todo make use of the Aria2C pub/sub events to make this quicker
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	for {
		status, err := cl.AriaClient.GetStatus(gid)
		if err != nil {
			log.Warnf("Error getting status for download %s: %s", gid, err)
		} else {
			if status.Status == "complete" {
				log.Infof("Download finished %s (%s)", status.GID, proposal.ProposalID)
				err := cl.BoostClient.ImportDeal(ctx, proposal, status.Files[0].Path)
				if err != nil {
					log.Warnf("Failure importing boost deal %s: %s", proposal.ProposalID, err)
					return
				}
				//log.Infof("Handled deal %s (%s) - removing from Aria2c", status.GID, proposal.ProposalID)

				// Remove from aria2c
				err = cl.AriaClient.RemoveDownload(status.GID)
				if err != nil {
					log.Warnf("Could not remove download from Aria2C: %s", err)
				}
				//log.Infof("Removed %s (%s) from aria2c", status.GID, proposal.ProposalID)

				// remove from actual list
				cl.ActiveDealsMutex.Lock()
				delete(cl.ActiveDeals, proposal.ProposalID)
				cl.ActiveDealsMutex.Unlock()

				// also add to imported list
				cl.AddImported(proposal.ProposalID)

				log.Infof("Successfully downloaded and imported %s", proposal.ProposalID)
				return
			}

			percentage := 0.0
			if status.CompletedLength != 0 && status.TotalLength != 0 {
				percentage = (float64(status.CompletedLength) / float64(status.TotalLength)) * 100.0
			}
			log.Infof("Download %s (%s): %s @ %s/s [%s / %s (%3.2f%%)]", status.GID, proposal.ProposalID, status.Status, humanize.Bytes(uint64(status.DownloadSpeed)), humanize.Bytes(uint64(status.CompletedLength)), humanize.Bytes(uint64(status.TotalLength)), percentage)
		}

		select {
		case <-ticker.C: // Return back into the loop
		case <-ctx.Done():
			log.Infof("Stopping download monitoring %s: context done", gid)
			return
		}
	}
}
