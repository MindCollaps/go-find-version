# go-find-version

**go-find-version** is a utility that helps you analyze and track files across the history of a Git repository. It is especially useful for security assessments or version detection when a web server exposes Git-based project files.

---

## Features

- **Fetch All Historical Files:**  
  Scans every commit in every branch of a Git repository and collects all unique file paths that ever existed.
- **Remote File Accessibility Check:**  
  Attempts to access files on a remote web server to determine which files are publicly available.
- **Version Detection:**  
  Helps identify between which commits files are published on a remote server, useful for detecting framework or application versions.
- **File Filtering:**  
  Supports filtering by file extensions (e.g., `.php`, `.js`) to focus on relevant files.
- **Progress Tracking:**  
  Provides a real-time progress bar and status updates for both repository scanning and remote file checks.
- **Save Results:**  
  Allows you to save the enumerated file list for later use, avoiding the need to re-scan the repository.

---

## Use Cases

- **Security Assessments:**  
  Detect which files and versions are exposed by a web server using Git-based projects.
- **Version Fingerprinting:**  
  Identify the exact version or commit range of a framework or application based on file availability.
- **Repository Analysis:**  
  Analyze the evolution of files across branches and commits for forensic or auditing purposes.

---

## Quick Start

1. **Clone the repository and build:**

```
git clone https://github.com/MindCollaps/go-find-version.git
cd go-find-version
go build
```

2. **Run the tool:**
```
go-find-version -g <REPO_URL> -w <WEBSITE_URL>
```

3. **Filter files (optional):**
```
go-find-version -g <REPO_URL> -w <WEBSITE_URL> --filter .php,.js
```

4. **Monitor progress:**

The tool will display progress bars and status updates in the terminal.

5. **Save results:**

The enumerated file list is saved with a timestamp and repository details for future reference.

---