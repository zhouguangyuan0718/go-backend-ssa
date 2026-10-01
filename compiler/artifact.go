package compiler

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"go-backend-ssa/gcbridge"
	"go/version"
	"io"
	"runtime"
	"slices"

	"go-backend-ssa/internal/go/buildcfg"
	"go-backend-ssa/internal/go/pkgbits"
)

const artifactMagic = "GOSSAART\x00"
const ArtifactFormat = 1
const maxArtifactHeader = 1 << 20

// ArtifactDependency binds a direct import to its exact input representation.
// Kind is "artifact" or "native-archive". The latter is an explicit adapter.
type ArtifactDependency struct {
	Path   string
	Kind   string
	SHA256 [sha256.Size]byte
}

// ArtifactInfo describes an immutable, versioned frontend import artifact.
// Metadata.Target.Experiments is canonical, matching Metadata.Experiments.
type ArtifactInfo struct {
	// NativeObjectReferences marks a dependency closure containing native archives.
	// Such inputs retain their original object identity and require those objects at link time.
	NativeObjectReferences  bool
	Format                  int
	ImportPath, PackageName string
	Metadata                Metadata
	Dependencies            []ArtifactDependency
}

// Artifact owns native Unified IR types and function bodies wrapped in a strict
// library header and SHA-256 checksum. It contains no machine code. An artifact-only dependency
// closure uses name-based symbols; explicit native archive adapters may retain
// references to their original object indices. Its immutable contents can be shared by concurrent builds.
type Artifact struct {
	info    ArtifactInfo
	payload string
	encoded []byte
	hash    [sha256.Size]byte
}

func (a *Artifact) Info() ArtifactInfo {
	if a == nil {
		return ArtifactInfo{}
	}
	info := a.info
	info.Dependencies = append([]ArtifactDependency(nil), info.Dependencies...)
	return info
}
func (a *Artifact) Bytes() []byte {
	if a == nil {
		return nil
	}
	return bytes.Clone(a.encoded)
}
func (a *Artifact) SHA256() [sha256.Size]byte {
	if a == nil {
		return [sha256.Size]byte{}
	}
	return a.hash
}

// DecodeArtifact copies and checks the format, precise frontend pin, canonical
// metadata and whole-container checksum. Build additionally checks compatibility
// with its request and the identities of all declared dependency inputs.
func DecodeArtifact(data []byte) (*Artifact, error) {
	const prefix = len(artifactMagic) + 4
	if len(data) < prefix+sha256.Size || string(data[:len(artifactMagic)]) != artifactMagic {
		return nil, fmt.Errorf("invalid SSA artifact header")
	}
	n := uint64(binary.LittleEndian.Uint32(data[len(artifactMagic):prefix]))
	end := uint64(len(data) - sha256.Size)
	if n == 0 || n > maxArtifactHeader || uint64(prefix)+n > end {
		return nil, fmt.Errorf("invalid SSA artifact header length")
	}
	sum := sha256.Sum256(data[:end])
	if !bytes.Equal(sum[:], data[end:]) {
		return nil, fmt.Errorf("SSA artifact checksum mismatch")
	}
	header := data[prefix : uint64(prefix)+n]
	var info ArtifactInfo
	decoder := json.NewDecoder(bytes.NewReader(header))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&info); err != nil {
		return nil, fmt.Errorf("SSA artifact metadata: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("trailing SSA artifact metadata")
	}
	canonical, err := json.Marshal(info)
	if err != nil || !bytes.Equal(canonical, header) {
		return nil, fmt.Errorf("noncanonical SSA artifact metadata")
	}
	if err := validateArtifactInfo(info); err != nil {
		return nil, err
	}
	payload := string(data[uint64(prefix)+n : end])
	// The native decoder requires a header and an eight-byte fingerprint. The
	// checksum covers the entire payload, including those native format records.
	if err := validateArtifactPayload(payload); err != nil {
		return nil, err
	}
	if err := validateArtifactIdentity(info, payload); err != nil {
		return nil, err
	}
	return &Artifact{info: info, payload: payload, encoded: bytes.Clone(data), hash: sha256.Sum256(data)}, nil
}

func validateArtifactInfo(info ArtifactInfo) error {
	m := info.Metadata
	if info.Format != ArtifactFormat || m.UpstreamVersion != Version || m.UpstreamCommit != UpstreamCommit {
		return fmt.Errorf("incompatible SSA artifact format or frontend pin")
	}
	if info.ImportPath == "" || info.ImportPath == "unsafe" || info.PackageName == "" || (m.Stage != Built && m.Stage != EarlyOptimized) {
		return fmt.Errorf("invalid SSA artifact package or stage")
	}
	if !version.IsValid(m.Language) || version.Lang(m.Language) != m.Language || version.Compare(m.Language, "go1.27") > 0 {
		return fmt.Errorf("unsupported SSA artifact language %q", m.Language)
	}
	canonical, _, targetErr := normalizeTarget(m.Target)
	if targetErr != nil || canonical != m.Target {
		return fmt.Errorf("unsupported or noncanonical SSA artifact target")
	}
	exp, err := buildcfg.ParseGOEXPERIMENT(m.Target.GOOS, m.Target.GOARCH, m.Experiments)
	if err != nil || exp.String() != m.Experiments || m.Target.Experiments != m.Experiments {
		return fmt.Errorf("noncanonical SSA artifact experiments")
	}
	previous := ""
	for _, dep := range info.Dependencies {
		if dep.Path == "" || dep.Path == "unsafe" || dep.Path == info.ImportPath || dep.Path <= previous || (dep.Kind != "artifact" && dep.Kind != "native-archive") {
			return fmt.Errorf("invalid SSA artifact dependency")
		}
		if dep.Kind == "native-archive" && !info.NativeObjectReferences {
			return fmt.Errorf("SSA artifact omits native object reference capability")
		}
		previous = dep.Path
	}
	return nil
}

func packageArtifact(req Request, p *Package, payload string) (*Artifact, error) {
	info := ArtifactInfo{Format: ArtifactFormat, ImportPath: p.ImportPath, PackageName: p.Name, Metadata: p.Metadata}
	info.Metadata.Target.Experiments = info.Metadata.Experiments
	for _, dep := range p.Imports {
		if dep.Path == "unsafe" {
			continue
		}
		d := ArtifactDependency{Path: dep.Path, Kind: "native-archive", SHA256: dep.ArchiveSHA256}
		if dep.ArtifactSHA256 != ([sha256.Size]byte{}) {
			d.Kind = "artifact"
			d.SHA256 = dep.ArtifactSHA256
			if req.Artifacts[dep.Path].info.NativeObjectReferences {
				info.NativeObjectReferences = true
			}
		}
		if d.Kind == "native-archive" {
			info.NativeObjectReferences = true
		}
		info.Dependencies = append(info.Dependencies, d)
	}
	slices.SortFunc(info.Dependencies, func(a, b ArtifactDependency) int { return compareString(a.Path, b.Path) })
	if err := validateArtifactInfo(info); err != nil {
		return nil, err
	}
	header, err := json.Marshal(info)
	if err != nil {
		return nil, err
	}
	if len(header) > maxArtifactHeader {
		return nil, fmt.Errorf("SSA artifact header too large")
	}
	encoded := []byte(artifactMagic)
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(len(header)))
	encoded = append(encoded, header...)
	encoded = append(encoded, payload...)
	checksum := sha256.Sum256(encoded)
	encoded = append(encoded, checksum[:]...)
	return &Artifact{info: info, payload: payload, encoded: encoded, hash: sha256.Sum256(encoded)}, nil
}

func validateArtifactInputs(req Request, language, experiments string, resolve func(string) ([]byte, bool, error)) error {
	paths := make([]string, 0, len(req.Artifacts))
	for path := range req.Artifacts {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	for _, path := range paths {
		a := req.Artifacts[path]
		if a == nil || len(a.encoded) == 0 {
			return fmt.Errorf("missing SSA artifact for %q", path)
		}
		m := a.info.Metadata
		if path != a.info.ImportPath || m.Language != language || m.Target.GOOS != req.Target.GOOS || m.Target.GOARCH != req.Target.GOARCH || m.Target.GOAMD64 != req.Target.GOAMD64 || m.Target.GOARM64 != req.Target.GOARM64 || m.Experiments != experiments || m.Race != req.Options.Race || m.Shared != (req.Options.Shared || req.Options.DynamicLink) || m.DynamicLink != req.Options.DynamicLink {
			return fmt.Errorf("SSA artifact %q is incompatible with the requested package, language, target, experiments, instrumentation or shared-code mode", path)
		}
		if _, ok := req.Imports[path]; ok {
			return fmt.Errorf("dependency %q has both artifact and native archive inputs", path)
		}
		nativeReferences := false
		for _, dep := range a.info.Dependencies {
			var hash [sha256.Size]byte
			if dep.Kind == "artifact" {
				if d := req.Artifacts[dep.Path]; d != nil {
					hash = d.hash
					nativeReferences = nativeReferences || d.info.NativeObjectReferences
				}
			} else {
				d, ok, loadErr := resolve(dep.Path)
				if loadErr != nil {
					return fmt.Errorf("SSA artifact %q dependency %q: %w", path, dep.Path, loadErr)
				}
				if ok {
					hash = sha256.Sum256(d)
					nativeReferences = true
				}
			}
			if hash != dep.SHA256 || hash == ([sha256.Size]byte{}) {
				return fmt.Errorf("SSA artifact %q dependency %q is missing or has a different identity", path, dep.Path)
			}
		}
		if nativeReferences != a.info.NativeObjectReferences {
			return fmt.Errorf("SSA artifact %q has inconsistent native object reference capability", path)
		}
	}
	return nil
}

// Check the native decoder's allocation and slicing boundaries before entering
// it. These are format checks, not an alternative Unified IR decoding path.
func validateArtifactPayload(payload string) error {
	const sections = int(pkgbits.SectionBody) + 1
	const fixed = 8 + 4*sections
	if len(payload) < fixed+8 {
		return fmt.Errorf("truncated SSA artifact payload")
	}
	data := []byte(payload)
	if binary.LittleEndian.Uint32(data) != uint32(pkgbits.V4) || binary.LittleEndian.Uint32(data[4:]) != 0 {
		return fmt.Errorf("unsupported SSA artifact Unified IR format")
	}
	sum := sha256.Sum256(data[:len(data)-8])
	if !bytes.Equal(sum[:8], data[len(data)-8:]) {
		return fmt.Errorf("SSA artifact Unified IR fingerprint mismatch")
	}
	var ends [sections]uint32
	var previous uint32
	for i := range ends {
		ends[i] = binary.LittleEndian.Uint32(data[8+i*4:])
		if ends[i] < previous {
			return fmt.Errorf("invalid SSA artifact section index")
		}
		previous = ends[i]
	}
	count := uint64(previous)
	if count == 0 || count > uint64(len(data)-fixed-8)/4 {
		return fmt.Errorf("invalid SSA artifact element count")
	}
	if ends[pkgbits.SectionMeta]-ends[pkgbits.SectionMeta-1] < 2 || ends[pkgbits.SectionPkg] == ends[pkgbits.SectionPkg-1] {
		return fmt.Errorf("SSA artifact lacks package roots")
	}
	start := uint64(fixed) + 4*count
	size := uint64(len(data)) - start - 8
	previous = 0
	for i := uint64(0); i < count; i++ {
		end := binary.LittleEndian.Uint32(data[uint64(fixed)+4*i:])
		if end < previous || uint64(end) > size {
			return fmt.Errorf("invalid SSA artifact element extent")
		}
		previous = end
	}
	if uint64(previous) != size {
		return fmt.Errorf("trailing SSA artifact Unified IR data")
	}
	// Bound relocation-table allocation and validate all table references before
	// native lazy decoding. String elements have no relocation prefix.
	for i := uint64(ends[pkgbits.SectionString]); i < count; i++ {
		lo := uint32(0)
		if i != 0 {
			lo = binary.LittleEndian.Uint32(data[uint64(fixed)+4*(i-1):])
		}
		hi := binary.LittleEndian.Uint32(data[uint64(fixed)+4*i:])
		r := bytes.NewReader(data[start+uint64(lo) : start+uint64(hi)])
		n, err := binary.ReadUvarint(r)
		if err != nil || n > uint64(r.Len()/2) {
			return fmt.Errorf("invalid SSA artifact relocation count")
		}
		for j := uint64(0); j < n; j++ {
			kind, e1 := binary.ReadUvarint(r)
			index, e2 := binary.ReadUvarint(r)
			if e1 != nil || e2 != nil || kind >= uint64(sections) {
				return fmt.Errorf("invalid SSA artifact relocation")
			}
			begin := uint32(0)
			if kind != 0 {
				begin = ends[kind-1]
			}
			if index >= uint64(ends[kind]-begin) {
				return fmt.Errorf("SSA artifact relocation index out of bounds")
			}
		}
	}
	return nil
}

func validateArtifactIdentity(info ArtifactInfo, payload string) (err error) {
	// This isolated decoder has no host callbacks or mutable compiler state.
	// Convert its recognized malformed-input panics into artifact errors.
	defer func() {
		if p := recover(); p != nil {
			switch v := p.(type) {
			case pkgbits.DecodeAbort, runtime.Error, error:
				err = fmt.Errorf("invalid SSA artifact package identity: %v", v)
			case string:
				if v != "assertion failed" {
					panic(p)
				}
				err = fmt.Errorf("invalid SSA artifact package identity")
			default:
				panic(p)
			}
		}
	}()
	decoder := pkgbits.NewPkgDecoder(info.ImportPath, payload)
	root := decoder.NewDecoder(pkgbits.SectionMeta, pkgbits.PublicRootIdx, pkgbits.SyncPublic)
	root.Sync(pkgbits.SyncPkg)
	pkg := decoder.NewDecoder(pkgbits.SectionPkg, root.Reloc(pkgbits.SectionPkg), pkgbits.SyncPkgDef)
	path, name := pkg.String(), pkg.String()
	if path != info.ImportPath || name != info.PackageName {
		return fmt.Errorf("SSA artifact package identity differs from its payload")
	}
	return nil
}

// UnifiedIR returns a copy of the exact pinned native frontend payload for
// backend object writers. Persisted library imports use Bytes and DecodeArtifact.
func (a *Artifact) UnifiedIR() []byte {
	if a == nil {
		return nil
	}
	return []byte(a.payload)
}

// Fingerprint is the Go linker identity of the Unified IR payload. Objects
// implementing the native Go adapter must use this identity for the package.
func (a *Artifact) Fingerprint() gcbridge.Fingerprint {
	var result gcbridge.Fingerprint
	if a != nil && len(a.payload) >= len(result) {
		copy(result[:], a.payload[len(a.payload)-len(result):])
	}
	return result
}
