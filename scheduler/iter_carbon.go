package scheduler

import (
	"strconv"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/nomad/structs"
)

type NoopIterator struct {
	source RankIterator
}

func (iter *NoopIterator) Next() *RankedNode {
	return iter.source.Next()
}

func (iter *NoopIterator) Reset() {
	iter.source.Reset()
}

type CarbonScoreIterator struct {
	max    float64
	def    float64
	ctx    Context
	source RankIterator
	logger hclog.Logger
}

func NewCarbonScoreIterator(ctx Context, source RankIterator, schedConfig *structs.SchedulerConfiguration) RankIterator {

	// Disable carbon scoring
	if schedConfig.CarbonMaxScore == 0 {
		ctx.Logger().Named("carbon").Info("Carbon scoring disabled; please set carbon_max_score > 0 to enable")

		return &NoopIterator{source: source}
	}

	return &CarbonScoreIterator{
		ctx:    ctx,
		source: source,
		logger: ctx.Logger().Named("carbon"),
	}
}

// Next yields a ranked option or nil if exhausted
func (c *CarbonScoreIterator) Next() *RankedNode {
	option := c.source.Next()
	if option == nil {
		return nil
	}

	score := c.def
	strScore := option.Node.Meta["carbon_score"]
	if strScore == "" {
		//TODO(carbon) No carbon, set default?
		return option
	}

	score, err := strconv.ParseFloat(strScore, 64)
	if err != nil {
		//TODO(carbon) don't log every time we hit an invalid node
		c.logger.Error("invalid carbon score; must be a float", "raw", strScore, "error", err)
		score = c.def
	}

	// Normalize score
	score /= c.max

	// More carbon == worse score
	score *= -1.0

	// Enforce bounds
	if score < -1.0 {
		score = -1.0
	} else if score > 0 {
		score = 0
	}

	option.Scores = append(option.Scores, score)
	c.ctx.Metrics().ScoreNode(option.Node, "carbon", score)
	return option
}

// Reset is invoked when an allocation has been placed
// to reset any stale state.
func (c *CarbonScoreIterator) Reset() {
	c.source.Reset()
}
