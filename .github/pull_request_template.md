## 📌 Description

<!-- Provide a brief summary of the changes introduced by this PR. Include motivation and context. -->

## 🔗 Related Issues

<!-- Reference any related issue(s) here. E.g., Fixes #123, Closes #456 -->
- Fixes #

## 📋 Type of Change

- [ ] 🐛 Bug fix (non-breaking change which fixes an issue)
- [ ] ✨ New feature (non-breaking change which adds functionality)
- [ ] 🛡️ Security patch / vulnerability mitigation
- [ ] ⚡ Performance improvement / zero-allocation optimization
- [ ] 📝 Documentation update
- [ ] 🧪 Testing / CI workflow improvement
- [ ] 💥 Breaking change (fix or feature that would cause existing functionality to not work as expected)

## 🧪 Verification & Testing

<!-- Describe how this was tested. Include details of test environment, commands run, and output. -->

- [ ] All unit tests pass with race detector: `go test -v -race ./...`
- [ ] Static analysis clean: `go vet ./...`
- [ ] Zero-allocation verified (if touching core hot-paths): `go test -bench=. -benchmem ./...`
- [ ] Added/updated tests covering the changes

## 📝 Checklist

- [ ] My code adheres to the idiomatic Go style and project guidelines in [CONTRIBUTING.md](CONTRIBUTING.md)
- [ ] I have commented my code, particularly in hard-to-understand areas
- [ ] I have updated relevant documentation / README / CHANGELOG if needed
- [ ] No data race or memory leak introduced
