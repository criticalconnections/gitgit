package main

import "syscall"

// diskUsage reports free and total bytes on the filesystem holding path.
func diskUsage(path string) (free, total uint64, err error) {
	var st syscall.Statfs_t
	if err = syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	return st.Bfree * uint64(st.Bsize), st.Blocks * uint64(st.Bsize), nil
}
