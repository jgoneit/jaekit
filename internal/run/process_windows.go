package run

// Windows checks enter a kill-on-close Job Object before their primary thread
// is resumed. This retains ordinary descendants without relying on a shell or
// a racy snapshot of child process IDs. Windows 11 x64 is the supported target.

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	createJob    = kernel32.NewProc("CreateJobObjectW")
	setJob       = kernel32.NewProc("SetInformationJobObject")
	assignJob    = kernel32.NewProc("AssignProcessToJobObject")
	terminateJob = kernel32.NewProc("TerminateJobObject")
	queryJob     = kernel32.NewProc("QueryInformationJobObject")
	threadFirst  = kernel32.NewProc("Thread32First")
	threadNext   = kernel32.NewProc("Thread32Next")
	openThread   = kernel32.NewProc("OpenThread")
	resumeThread = kernel32.NewProc("ResumeThread")
)

type jobBasicLimits struct {
	ProcessTime, JobTime                 int64
	Flags                                uint32
	MinimumWorkingSet, MaximumWorkingSet uintptr
	ActiveProcessLimit                   uint32
	Affinity                             uintptr
	PriorityClass, SchedulingClass       uint32
}

type jobExtendedLimits struct {
	Basic                                                      jobBasicLimits
	IO                                                         [6]uint64
	ProcessMemory, JobMemory, PeakProcessMemory, PeakJobMemory uintptr
}

type jobAccounting struct {
	UserTime, KernelTime, PeriodUserTime, PeriodKernelTime           int64
	PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
}

type threadEntry struct {
	Size, Usage, ID, Owner      uint32
	BasePriority, DeltaPriority int32
	Flags                       uint32
}

type processGroup struct {
	job syscall.Handle
	// accounting is the job's last accounting after every process ended.
	accounting *jobAccounting
}

func winCall(proc *syscall.LazyProc, args ...uintptr) error {
	ok, _, err := proc.Call(args...)
	return winResult(proc, ok, err)
}

func winResult(proc *syscall.LazyProc, ok uintptr, err error) error {
	if ok != 0 {
		return nil
	}
	if errors.Is(err, syscall.Errno(0)) {
		return fmt.Errorf("%s failed", proc.Name)
	}
	return fmt.Errorf("%s: %w", proc.Name, err)
}

func startProcess(cmd *exec.Cmd) (processController, error) {
	h, _, err := createJob.Call(0, 0)
	if h == 0 {
		return nil, fmt.Errorf("CreateJobObjectW: %w", err)
	}
	g := &processGroup{job: syscall.Handle(h)}
	limits := jobExtendedLimits{}
	limits.Basic.Flags = 0x2000 // JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE; no breakaway.
	ok, _, setErr := setJob.Call(h, 9, uintptr(unsafe.Pointer(&limits)), unsafe.Sizeof(limits))
	if err := winResult(setJob, ok, setErr); err != nil {
		syscall.CloseHandle(g.job)
		return nil, err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000004} // CREATE_SUSPENDED
	if err := cmd.Start(); err != nil {
		syscall.CloseHandle(g.job)
		return nil, err
	}
	// os/exec closes the primary thread handle. Locate that still-suspended
	// thread only after assigning the process to the job; no user code has run.
	p, err := syscall.OpenProcess(0x0100|0x0001, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = winCall(assignJob, h, uintptr(p))
		syscall.CloseHandle(p)
	}
	if err == nil {
		err = resumePrimary(uint32(cmd.Process.Pid))
	}
	if err != nil {
		// On admission failure no checker is allowed to run without containment.
		// Close the kill-on-close job before waiting, including when resuming
		// the primary thread failed. Parent-only termination may itself fail.
		jobErr := g.kill()
		closeErr := syscall.CloseHandle(g.job)
		killErr := cmd.Process.Kill()
		waited := make(chan error, 1)
		go func() { waited <- cmd.Wait() }()
		select {
		case <-waited:
		case <-time.After(5 * time.Second):
			err = errors.Join(err, errors.New("admission cleanup did not terminate the process"))
		}
		// Conservatively withhold a final digest after any post-start admission
		// error; cleanup attempts are bounded and are not a completion proof.
		err = errors.Join(errUnfinalizedProcess, err, jobErr, closeErr, killErr)
		return nil, err
	}
	return g, nil
}

func resumePrimary(pid uint32) error {
	snapshot, err := syscall.CreateToolhelp32Snapshot(0x00000004, 0) // TH32CS_SNAPTHREAD
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(snapshot)
	entry := threadEntry{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	more, _, err := threadFirst.Call(uintptr(snapshot), uintptr(unsafe.Pointer(&entry)))
	for more != 0 {
		if entry.Owner == pid {
			thread, _, openErr := openThread.Call(0x0002, 0, uintptr(entry.ID)) // THREAD_SUSPEND_RESUME
			if thread == 0 {
				return fmt.Errorf("OpenThread: %w", openErr)
			}
			previous, _, resumeErr := resumeThread.Call(thread)
			syscall.CloseHandle(syscall.Handle(thread))
			if uint32(previous) == 0xffffffff {
				return fmt.Errorf("ResumeThread: %w", resumeErr)
			}
			if previous != 1 {
				return fmt.Errorf("unexpected primary thread suspend count %d", previous)
			}
			return nil
		}
		entry.Size = uint32(unsafe.Sizeof(entry))
		more, _, err = threadNext.Call(uintptr(snapshot), uintptr(unsafe.Pointer(&entry)))
	}
	return fmt.Errorf("primary thread unavailable for suspended process: %w", err)
}

func (g *processGroup) kill() error { return winCall(terminateJob, uintptr(g.job), 1) }

func (g *processGroup) finish() error {
	defer syscall.CloseHandle(g.job)
	// A checker cannot leave an ordinary child writing the recorded output
	// after the parent's exit. Confirm termination before hashing that output.
	if err := g.kill(); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var accounting jobAccounting
		ok, _, queryErr := queryJob.Call(uintptr(g.job), 1, uintptr(unsafe.Pointer(&accounting)), unsafe.Sizeof(accounting), 0)
		if err := winResult(queryJob, ok, queryErr); err != nil {
			return err
		}
		if accounting.ActiveProcesses == 0 {
			g.accounting = &accounting
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("job processes did not terminate")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func processSignal(_ *exec.ExitError) (int, string) { return 0, "" }

// processCPU reads the user and kernel time the Job Object accounted for every
// process that ran in the job, read after all of them ended.
func processCPU(_ *exec.Cmd, controller processController) CPU {
	cpu := CPU{Status: CPUFailed, Scope: "job-process-tree", Source: "job-object-accounting"}
	g, ok := controller.(*processGroup)
	if !ok || g.accounting == nil {
		return cpu
	}
	// Job accounting counts in 100-nanosecond units.
	cpu.Status = CPUMeasured
	cpu.User = time.Duration(g.accounting.UserTime) * 100
	cpu.System = time.Duration(g.accounting.KernelTime) * 100
	return cpu
}
