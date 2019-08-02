package simple

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/cel-spec/tools/celrpc"

	spb "github.com/google/cel-spec/proto/test/v1/simple"
)

var (
	flagServerCmd      string
	flagParseServerCmd string
	flagCheckServerCmd string
	flagEvalServerCmd  string
	flagSkipTestCmd    string
	rc                 *runConfig
)

func init() {
	flag.StringVar(&flagServerCmd, "server", "", "path to binary for server when no phase-specific server defined")
	flag.StringVar(&flagParseServerCmd, "parse_server", "", "path to binary for parse server")
	flag.StringVar(&flagCheckServerCmd, "check_server", "", "path to binary for check server")
	flag.StringVar(&flagEvalServerCmd, "eval_server", "", "path to binary for eval server")
	flag.StringVar(&flagSkipTestCmd, "skip_test", "", "name(s) of tests to skip in the format [name](/section(/test)?)?, multiple tests in same section delimited by , and different filenames delimited by ;")
	flag.Parse()
}

// Server binaries specified by flags
func initRunConfig() (*runConfig, error) {
	// Find the server binary for each phase
	pCmd := flagServerCmd
	if flagParseServerCmd != "" {
		pCmd = flagParseServerCmd
	}
	if pCmd == "" {
		return nil, fmt.Errorf("no parse server defined")
	}

	cCmd := flagServerCmd
	if flagCheckServerCmd != "" {
		cCmd = flagCheckServerCmd
	}
	if cCmd == "" {
		return nil, fmt.Errorf("no check server defined")
	}

	eCmd := flagServerCmd
	if flagEvalServerCmd != "" {
		eCmd = flagEvalServerCmd
	}
	if eCmd == "" {
		return nil, fmt.Errorf("no eval server defined")
	}

	// Only launch each required binary once
	servers := make(map[string]*celrpc.ConfClient)
	servers[pCmd] = nil
	servers[cCmd] = nil
	servers[eCmd] = nil
	for cmd := range servers {
		cli, err := celrpc.NewClientFromPath(cmd)
		if err != nil {
			return nil, err
		}
		servers[cmd] = cli
	}

	var rc runConfig
	rc.parseClient = servers[pCmd]
	rc.checkClient = servers[cCmd]
	rc.evalClient = servers[eCmd]
	return &rc, nil
}

// File path specified by flag
// TODO(jimlarson) use the utility filter to do text to binary
// proto conversion, since the C++ implementation understands Any
// messages.
func parseSimpleFile(filename string) (*spb.SimpleTestFile, error) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	s := string(bytes)
	var pb spb.SimpleTestFile
	err = proto.UnmarshalText(s, &pb)
	if err != nil {
		return nil, err
	}
	return &pb, nil
}

// Usage: --server=<path-to-binary> testfile1 ...
func TestMain(m *testing.M) {
	flag.Parse()
	var err error
	// When flags are specified construct the run config object. When no
	// flags have been specified, the run config will be nil and the tests
	// will early return.
	if len(os.Args) > 1 {
		rc, err = initRunConfig()
	}
	if err != nil {
		// Silly Go has no method in M to log errors or abort,
		// so we'll have to do it outside of the testing module.
		log.Fatal("Can't initialize test server", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestSimpleFile(t *testing.T) {
        // Special case to handle test invocation without args. See TestMain()
        // early return comment.
        if rc == nil {
                return
        }
        testArray := strings.Split(flagSkipTestCmd, ";")
        m := make(map[string]string)
        for _, fullTest := range testArray {
                fmt.Println("skipping test", fullTest)
                section := strings.Index(fullTest, "/")
                if  (section != -1) {
                        m[fullTest[:section]] = fullTest[section+1:]
                } else {
                        m[fullTest] = "ALL"
                }
        }
        // Run the flag-configured tests.
        for _, filename := range flag.Args() {
                testFile, err := parseSimpleFile(filename)
                if err != nil {
                        t.Fatalf("Can't parse input file %v: %v", filename, err)
                }
                if _, skip := m[testFile.Name]; skip {
                        if m[testFile.Name] == "ALL"{
                                t.Logf("Skipping all tests in filename %v\n", testFile.Name)
                        } else {
                                t.Logf("Running tests in file %v\n", testFile.Name)
                                subtest := m[testFile.Name]
				ind := strings.Index(subtest, "/")
				var testName string
                                if ind != -1 {
					testName = subtest[ind+1:]
                                        subtest = subtest[:ind]
                                }
                                for _, section := range testFile.Section {
                                        if subtest == section.Name {
                                                if ind == -1 {
                                                        t.Logf("Skipping all tests in section %v\n", section.Name)
                                                } else {
							tests := strings.Split(testName, ",")
                                                        t.Logf("Running tests in section %v\n", section.Name)
                                                        for _, test := range section.Test {
                                                                if contains(tests, test.Name){
									t.Logf("Skipping test name %v\n", test.Name)
                                                                } else {
                                                                        desc := fmt.Sprintf("%s/%s/%s", testFile.Name, section.Name, test.Name)
                                                                        t.Run(desc, func(t *testing.T) {
                                                                                err := rc.RunTest(test)
                                                                                if err != nil {
                                                                                        t.Fatal(err)
                                                                                }
                                                                        })
                                                                }
                                                        }
                                                }

                                        } else {
                                                t.Logf("Running tests in section %v\n", section.Name)
                                                for _, test := range section.Test {
                                                        desc := fmt.Sprintf("%s/%s/%s", testFile.Name, section.Name, test.Name)
                                                        t.Run(desc, func(t *testing.T) {
                                                                err := rc.RunTest(test)
                                                                if err != nil {
                                                                        t.Fatal(err)
                                                                }
                                                        })
                                                }
                                        }
                                }
                        }

                } else {
                        t.Logf("Running tests in file %v\n", testFile.Name)
                        for _, section := range testFile.Section {
                                t.Logf("Running tests in section %v\n", section.Name)
                                for _, test := range section.Test {
                                        desc := fmt.Sprintf("%s/%s/%s", testFile.Name, section.Name, test.Name)
                                        t.Run(desc, func(t *testing.T) {
                                                err := rc.RunTest(test)
                                                if err != nil {
                                                        t.Fatal(err)
                                                }
                                        })
                                }
                        }
                }
        }
}

func contains(s []string, e string) bool {
        for _, a := range s {
                if a == e {
                        return true
                }
        }
        return false
}
