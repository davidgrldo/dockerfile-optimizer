# 🐳 Dockerfile Optimizer

A powerful CLI tool that **analyzes Dockerfiles** for optimization and best practices — with **stack-aware detection** for Go, Python, Java, Node.js, Rust, PHP, .NET, and more! 💡

---

## 🚀 Features

- 🔍 **Auto-detects stack** (Go, Python, Node.js, Java, etc.)
- 🧠 **Lints Dockerfiles** using best practices
- ⚡ Fast and dependency-free (pure Go)
- 📦 CLI-friendly for CI/CD pipelines
- 🧰 Easily extensible rule engine

---

## 📦 Installation

```bash
git clone https://github.com/yourusername/dockerfile-optimizer.git
cd dockerfile-optimizer
go build -o dockopt ./cmd/dockopt
```
---

## 🧪 Usage
### 🔍 Analyze a Dockerfile

Example
```bash
./dockopt path/to/Dockerfile
```

Output
```bash
🔍 Detected stack: rust
🚨 Optimization Suggestions:
 - Consider using multi-stage builds in Rust to reduce final image size
```

---

## 💡 Supported Stacks
| Stack   | Detected By                   | Example Base Images        |
| ------- | ----------------------------- | -------------------------- |
| Golang  | `golang`, `go build`          | `golang:1.21`, `alpine`    |
| Python  | `python`, `pip install`       | `python:3.10`              |
| Node.js | `node`, `npm install`, `yarn` | `node:18-alpine`           |
| Java    | `java`, `openjdk`, `maven`    | `openjdk:17`, `gradle:8`   |
| Rust    | `rust`, `cargo`               | `rust:1.73`                |
| .NET    | `dotnet`, `csproj`            | `mcr.microsoft.com/dotnet` |
| PHP     | `php`, `composer`             | `php:8.2`                  |
| Ruby    | `ruby`, `bundle install`      | `ruby:3.2`                 |
| C/C++   | `gcc`, `make`, `cmake`        | `debian`, `alpine`         |

---

## 🧰 Rules Example

Each stack has its own set of rules. Example for Go:
- ✅ Use multi-stage builds
- ✅ Set CGO_ENABLED=0 for static builds
- ❌ Don’t use full golang: as final image

Other stacks include rules for:
- Version pinning
- Production install flags
- Caching package managers
- Reducing image layers

--- 

## 📁 Project Structure
```
dockerfile-optimizer/
├── cmd/
│   └── dockopt/        # CLI entrypoint
├── internal/
│   ├── parser/         # Parses Dockerfiles into []string
│   ├── analyzer/       # Rule engine and stack detection
│   └── report/         # Pretty prints results
```

---

## 🛠️ TODO / Roadmap
- [ ] --json output format for CI
- [ ] --stack=<name> manual override
- [ ] Rule severity levels (info/warn/error)
- [ ] Auto-fix mode (where possible)
- [ ] Web or TUI interface
- [ ] GitHub Action integration

---

## 🤝 Contributing

Want to add new rules or stacks?
Just edit:
- internal/analyzer/stacks.go → add your stack and keywords
- internal/analyzer/rules.go → write a Rule for it!

---

## 📄 License
MIT — do whatever you want. Just don’t ship insecure Dockerfiles 😄

---

## 👨‍💻 Made with Go and grit
Built by @davidgrldo with ❤️

```
Let me know if you want to:
- Add real badges (build status, license, version)
- Include GIF/demo in `docs/`
- Generate a changelog for versions (e.g. v0.2.0)

All yours now! 💥
```

---