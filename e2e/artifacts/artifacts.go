package artifacts

import (
	"io"
	"os"

	"github.com/hashicorp/nomad/e2e/e2eutil"
	"github.com/hashicorp/nomad/e2e/framework"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ArtifactsTest struct {
	framework.TC
	jobIDs []string
}

func init() {
	framework.AddSuites(&framework.TestSuite{
		Component:   "Artifacts",
		CanRunLocal: true,
		Cases: []framework.TestCase{
			new(ArtifactsTest),
		},
	})
}

func (tc *ArtifactsTest) BeforeAll(f *framework.F) {
	e2eutil.WaitForLeader(f.T(), tc.Nomad())
	e2eutil.WaitForNodesReady(f.T(), tc.Nomad(), 1)
}

func (tc *ArtifactsTest) AfterEach(f *framework.F) {
	if os.Getenv("NOMAD_TEST_SKIPCLEANUP") == "1" {
		return
	}

	for _, id := range tc.jobIDs {
		_, err := e2eutil.Command("nomad", "job", "stop", "-purge", id)
		f.Assert().NoError(err, "could not clean up job", id)
	}
	tc.jobIDs = []string{}

	_, err := e2eutil.Command("nomad", "system", "gc")
	f.NoError(err)
}

func (tc *ArtifactsTest) TestGitDepth(f *framework.F) {
	t := f.T()
	client := tc.Nomad()
	jobID := "deployment" + uuid.Generate()[0:8]
	tc.jobIDs = append(tc.jobIDs, jobID)
	stubs := e2eutil.RegisterAndWaitForAllocs(t, client, "artifacts/input/gitdepth.nomad", jobID, "")
	require.Len(t, stubs, 1)
	allocID := stubs[0].ID

	// Wait for batch job to complete
	alloc := e2eutil.WaitForAllocStopped(t, client, allocID)
	require.Equal(t, "complete", alloc.ClientStatus)

	// Read logs
	r, err := client.AllocFS().Cat(alloc, "/alloc/logs/gitdepth.stdout.0", nil)
	require.NoError(t, err)

	defer r.Close()

	// Expected 2 but capture more bytes in case of error
	logs := make([]byte, 120)
	n, err := r.Read(logs)
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, 2, n)

	// Assert only git log only has 1 entry
	assert.Equal(t, "1\n", string(logs[:2]), "full logs: %q", string(logs))
}
