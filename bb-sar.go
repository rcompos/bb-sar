package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/kyokomi/emoji"
	"github.com/rcompos/go-bitbucket"
)

const BBURL = "https://bitbucket.org"

//const sleepytime time.Duration = 2

func main() {

	rand.Seed(time.Now().UnixNano())

	var bbUser, bbPassword, bbOwner, bbRole, searchStr, replaceStr string
	var execute, createPR bool
	//var numWorkers int
	flag.StringVar(&bbUser, "u", os.Getenv("BITBUCKET_USERNAME"), "Bitbucket user (required) (envvar BITBUCKET_USERNAME)")
	flag.StringVar(&bbPassword, "p", os.Getenv("BITBUCKET_PASSWORD"), "Bitbucket password (required) (envvar BITBUCKET_PASSWORD)")
	flag.StringVar(&bbOwner, "o", os.Getenv("BITBUCKET_OWNER"), "Bitbucket owner (required) (envvar BITBUCKET_OWNER)")
	flag.StringVar(&bbRole, "e", os.Getenv("BITBUCKET_ROLE"), "Bitbucket role (envvar BITBUCKET_ROLE)")
	flag.StringVar(&searchStr, "s", os.Getenv("BITBUCKET_SEARCH"), "Text to search for (envvar BITBUCKET_SEARCH)")
	flag.StringVar(&replaceStr, "r", os.Getenv("BITBUCKET_REPLACE"), "Text to replace with (envvar BITBUCKET_REPLACE)")
	flag.BoolVar(&execute, "x", false, "Execute text replace")
	flag.BoolVar(&createPR, "c", false, "Create pull request")
	//flag.IntVar(&numWorkers, "w", 100, "Number of worker threads")
	flag.Parse()

	if bbUser == "" || bbPassword == "" || bbOwner == "" {
		fmt.Println("Must supply user (-u), password (-p) and owner (-o)!")
		fmt.Println("Alternately, environmental variables can be set.")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if execute == true && (searchStr == "" || replaceStr == "") {
		fmt.Println("Must supply search string (-s) and replace string (-r)!")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if execute {
		promptRead(bbOwner, searchStr, replaceStr)
	}

	logsDir := "logs"
	createDir(logsDir)
	logFileDateFormat := "2006-01-02-150405"
	logStamp := time.Now().Format(logFileDateFormat)
	logfile := logsDir + "/bb-sar-" + string(logStamp) + ".log"

	logf, err := os.OpenFile(logfile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0664)
	if err != nil {
		log.Fatal(err)
	}
	defer logf.Close()
	log.SetOutput(logf) //log.Println("Test log message")

	log.Printf("Current Unix Time: %v\n", time.Now().Unix())

	color.Set(color.FgMagenta)
	//fmt.Println(path.Base(os.Args[0]))
	emoji.Printf("Acquiring repos for %s [ clone :hamburger: | pull :fries: | untracked :gem: | pull request :thumbsup: ]\n\n", bbOwner)
	color.Unset() // Don't forget to unset
	//time.Sleep(2)

	opt := &bitbucket.RepositoriesOptions{}
	opt.Owner = bbOwner
	opt.Role = bbRole // [owner|admin|contributor|member]

	c := bitbucket.NewBasicAuth(bbUser, bbPassword)

	// TODO: File cache of repos

	repos, err := c.Repositories.ListForAccount(opt)
	if err != nil {
		panic(err)
	}

	repositories := repos.Items
	//fmt.Println("\nrepositories:\n", repositories)

	reposBaseDir := "repos"
	createDir(fmt.Sprintf("%s/%s", reposBaseDir, bbOwner))

	var repoList []string
	//for k, v := range repositories {
	for _, v := range repositories {
		//fmt.Println("\nk: %v	v: %v\n", k, v)
		//fmt.Printf("> %v  name:  %v\n", k, v.Full_name)
		repoList = append(repoList, v.Full_name)
		//fmt.Printf("> %v\nlinks: %v\n", k, v.Links)
	}

	var wg sync.WaitGroup

	// TODO: Change from waitgroup to buffered channels

	//fmt.Printf("Repos:\n")
	for i, j := range repoList {
		wg.Add(1)
		//fmt.Printf("%v> %v\n", i, j)
		go func(i int, j string, dir string, owner string, searchStr string, replaceStr string, createPR bool, user string, pw string) {
			defer wg.Done()
			dirOwner := dir + "/" + owner
			dirRepo := dir + "/" + j
			//fmt.Printf("%s\n", dirRepo)

			gitClone := "git clone " + BBURL + "/" + j
			errCloneNum := repoAction(j, gitClone, dirOwner, ":hamburger:", "", "", "")
			if errCloneNum == 128 {
				gitPull := "git pull"
				errPullNum := repoAction(j, gitPull, dirRepo, ":fries:", "", "", "")
				if errPullNum != 0 {
					emoji.Printf(":fork_and_knife:")
				}
			}

			featureBranch := "sf-artifactory-solidfire-net"
			branchCmd := "git checkout -b " + featureBranch
			branchExec := exec.Command("bash", "-c", branchCmd)
			branchExec.Dir = dirRepo
			branchExecOut, _ := branchExec.Output()
			bResult := string(branchExecOut)
			fmt.Printf(bResult)

			upstreamCmd := "git push --set-upstream origin " + featureBranch
			upstreamExec := exec.Command("bash", "-c", upstreamCmd)
			upstreamExec.Dir = dirRepo
			upstreamExec.Output()
			//upstreamExecOut, _ := upstreamExec.Output()
			//upstreamResult := string(upstreamExecOut)
			//fmt.Printf(upstreamResult)

			var sar string
			//# One-liner shell command will search and replace all files recursively.
			if execute == true {
				sar = `find . -path ./.git -prune -o -type f -print  -exec grep -Iq . {} \; -exec perl -i -pe"s/` +
					searchStr + `/` + replaceStr + `/g" {} \;`
			} else if searchStr != "" && replaceStr != "" {
				sar = `find . -path ./.git -prune -o -type f -print -exec grep -Iq . {} \; -exec perl -ne" print if s/` +
					searchStr + `/` + replaceStr + `/g" {} \;`
			} else if searchStr != "" {
				sar = `find . -path ./.git -prune -o -type f -print -exec grep -Iq . {} \; -exec perl -ne" print if /` +
					searchStr + `/g" {} \;`
			}

			if sar != "" {
				sarExec := exec.Command("bash", "-c", sar)
				sarExec.Dir = dirRepo
				sarExecOut, err := sarExec.Output()
				if err != nil {
					panic(err)
					fmt.Printf("ERROR: %v\n", err)
				}
				searchResult := string(sarExecOut)
				fmt.Printf(searchResult)
			}

			//fmt.Println("dirRepo: ", dirRepo)

			// Check for untracked changes
			gitDiffIndex := "git diff-index --quiet HEAD --"
			errPullNum := repoAction(j, gitDiffIndex, dirRepo, "", "", "", "")
			if errPullNum != 0 {
				// Git untracked changes exist
				emoji.Printf(":gem:")
				// create Pull Request

				if createPR == true {
					commitCmd := "git commit -am'Replace " + searchStr + " with " + replaceStr + "'"
					commitExec := exec.Command("bash", "-c", commitCmd)
					commitExec.Dir = dirRepo
					commitExec.Output()
					//commitExecOut, _ := commitExec.Output()
					//if err != nil {
					//	panic(err)
					//	fmt.Printf("ERROR: %v\n", err)
					//}
					//commitResult := string(commitExecOut)
					//fmt.Printf(commitResult)

					pushCmd := "git push"
					pushExec := exec.Command("bash", "-c", pushCmd)
					pushExec.Dir = dirRepo
					pushExec.Output()
					//pushExecOut, _ := pushExec.Output()
					//if err != nil {
					//	panic(err)
					//	fmt.Printf("ERROR: %v\n", err)
					//}
					//pushResult := string(pushExecOut)
					//fmt.Printf(pushResult)

					titlePR := "TEST-PR-TITLE " + j
					curlPR := fmt.Sprintf("curl -v https://api.bitbucket.org/2.0/repositories/%s/pullrequests "+
						"-u %s:%s --request POST --header 'Content-Type: application/json' "+
						"--data '{\"title\": \"%s\", \"source\": { \"branch\": { \"name\": \"%s\" } } }'", j, user, pw, titlePR, featureBranch)

					//fmt.Printf("curlPR:\n%s\n", curlPR)
					prExec := exec.Command("bash", "-c", curlPR)
					prExec.Dir = dirRepo
					_, err := prExec.Output()
					//prExecOut, err := prExec.Output()
					if err == nil {
						emoji.Printf(":thumbsup:")
					}
					//if err != nil {
					//	panic(err)
					//	fmt.Printf("ERROR: %v\n", err)
					//}
					//prResult := string(prExecOut)
					//fmt.Printf(prResult)
				}

			}

			//fmt.Printf("%vth goroutine done.\n", i)
		}(i, j, reposBaseDir, bbOwner, searchStr, replaceStr, createPR, bbUser, bbPassword)
	}

	wg.Wait()
	//fmt.Printf("\n\nAll goroutines complete.")
	fmt.Println()

} //

func repoAction(r string, cmdstr string, rdir string, win string, any string, fail string, fcess string) int {

	cmd := exec.Command("bash", "-c", cmdstr)
	cmd.Dir = rdir

	var errorNumber int = 0
	var waitStatus syscall.WaitStatus
	if err := cmd.Run(); err != nil {
		if err != nil {
			//os.Stderr.WriteString(fmt.Sprintf("Error: %s\n", err.Error()))
			log.Printf("Error: %s\n", err.Error())
			if fail != "" {
				emoji.Printf(fail)
			}
		}
		if exitError, ok := err.(*exec.ExitError); ok {
			waitStatus = exitError.Sys().(syscall.WaitStatus)
			errorNumber = waitStatus.ExitStatus()
			log.Printf("Output: %s\n", []byte(fmt.Sprintf("%d", waitStatus.ExitStatus())))
			if fcess != "" {
				emoji.Printf(fcess)
			}
		}
	} else {
		// Success
		waitStatus = cmd.ProcessState.Sys().(syscall.WaitStatus)
		errorNumber = waitStatus.ExitStatus()
		log.Printf("Output: %s\n", []byte(fmt.Sprintf("%d", waitStatus.ExitStatus())))
		if win != "" {
			emoji.Printf(win)
		}
	}
	if any != "" {
		emoji.Printf(any)
	}
	return errorNumber
}

func createDir(dir string) {
	//	if [ -d "repos" ]; then echo true; else echo false; fi
	mkdirCmd := fmt.Sprintf("if [ ! -d %s ]; then mkdir -p -m775 %s; fi", dir, dir)
	mkdirExec := exec.Command("bash", "-c", mkdirCmd)
	mkdirExecOut, err := mkdirExec.Output()
	if err != nil {
		panic(err)
	}
	//fmt.Println(mkdirCmd)
	fmt.Printf(string(mkdirExecOut))

}

func checkFileInfo(f string) {
	//fi, err := os.Lstat("some-filename")
	fi, err := os.Lstat(f)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("permissions: %#o\n", fi.Mode().Perm()) // 0400, 0777, etc.
	switch mode := fi.Mode(); {
	case mode.IsRegular():
		fmt.Println("regular file")
	case mode.IsDir():
		fmt.Println("directory")
	case mode&os.ModeSymlink != 0:
		fmt.Println("symbolic link")
	case mode&os.ModeNamedPipe != 0:
		fmt.Println("named pipe")
	}
}

func promptRead(owner string, s string, r string) {
	reader := bufio.NewReader(os.Stdin)
	if s == "" {
		fmt.Printf("Git clone all %s repos.\n", owner)
	} else {
		fmt.Printf("Perform text replacement in all %s repos.\n", owner)
		fmt.Printf("%s -> %s\n", s, r)
	}
	fmt.Printf("To continue type '%s': \n", owner)
	text, _ := reader.ReadString('\n')
	answer := strings.TrimRight(text, "\n")
	//fmt.Printf("answer: %s \n", answer)
	//if answer == "y" || answer == "Y" {
	if answer == owner {
		return
	} else {
		//prompt2() //For recursive prompting
		log.Fatal("Exiting without action.")
	}
}
