package difftool

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/Fantom-foundation/go-lachesis/src/node"
)

// Example illustrates nodes comparing
func Example() {
	logger := logrus.New()
	logger.Level = logrus.FatalLevel

	nodes, err := node.NewNodeList(3, logger)
	if err != nil {
		logger.Fatal(err)
	}

	stop := nodes.StartRandTxStream()
	nodes.WaitForBlock(5)
	stop()
	for _, n := range nodes {
		n.Shutdown()
	}
	// give some time for nodes to stop actually
	time.Sleep(time.Duration(5) * time.Second)

	diffResult := Compare(nodes.Values()...)

	if !diffResult.IsEmpty() {
		logger.Fatal("\n" + diffResult.ToString())
	}
	fmt.Println("all good")
	// Output:
	// all good
}
