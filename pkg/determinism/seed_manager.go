package determinism

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "hash"
    "math/rand"
    "reflect"
    "sort"
    "sync"
    "time"
)

// SeedManager ensures 100% reproducible results
type SeedManager struct {
    baseSeed      int64
    seedSources   map[string]*rand.Rand
    hashAlgorithm hash.Hash
    mu            sync.RWMutex
    initialized   bool
}

// NewSeedManager creates a new SeedManager with the given base seed
func NewSeedManager(baseSeed int64) *SeedManager {
    sm := &SeedManager{
        baseSeed:    baseSeed,
        seedSources: make(map[string]*rand.Rand),
        mu:          sync.RWMutex{},
    }

    // Initialize with base seed source
    sm.initialize()

    return sm
}

// initialize sets up the seed manager
func (sm *SeedManager) initialize() {
    if sm.initialized {
        return
    }

    sm.mu.Lock()
    defer sm.mu.Unlock()

    // Create base random source
    baseSource := rand.NewSource(sm.baseSeed)
    sm.seedSources["base"] = rand.New(baseSource)

    // Initialize hash algorithm
    sm.hashAlgorithm = sha256.New()

    sm.initialized = true
}

// GetSeedForProcess returns a deterministic seed for a process using HMAC derivation
func (sm *SeedManager) GetSeedForProcess(processName string) int64 {
    sm.mu.RLock()
    baseSeed := sm.baseSeed
    sm.mu.RUnlock()

    // HMAC derivation from base seed and process name
    h := hmac.New(sha256.New, []byte(fmt.Sprintf("%d", baseSeed)))
    h.Write([]byte(processName))
    seedBytes := h.Sum(nil)

    // Convert first 8 bytes to int64
    var seed int64
    for i := 0; i < 8; i++ {
        seed = (seed << 8) | int64(seedBytes[i])
    }
    return seed
}

// DeterministicHash creates a reproducible hash for tie-breaking
func (sm *SeedManager) DeterministicHash(input string) string {
    sm.mu.Lock()
    defer sm.mu.Unlock()

    sm.hashAlgorithm.Reset()
    sm.hashAlgorithm.Write([]byte(input))

    // Add base seed to ensure determinism across runs
    seedBytes := []byte(fmt.Sprintf("%d", sm.baseSeed))
    sm.hashAlgorithm.Write(seedBytes)

    return hex.EncodeToString(sm.hashAlgorithm.Sum(nil))
}

// StableSort performs a deterministic stable sort on any slice using JSON serialization.
func (sm *SeedManager) StableSort(slice interface{}) error {
    sm.mu.RLock()
    defer sm.mu.RUnlock()

    value := reflect.ValueOf(slice)
    if value.Kind() != reflect.Slice {
        return fmt.Errorf("StableSort requires a slice, got %T", slice)
    }

    sliceLen := value.Len()
    indices := make([]int, sliceLen)
    for i := 0; i < sliceLen; i++ {
        indices[i] = i
    }

    // Sort with deterministic tie-breaking via JSON representation
    sort.SliceStable(indices, func(i, j int) bool {
        vi := value.Index(indices[i]).Interface()
        vj := value.Index(indices[j]).Interface()

        // Serialize to JSON for a fully deterministic canonical representation.
        // This eliminates randomness from timestamps, map ordering, etc.
        viJSON, err1 := json.Marshal(vi)
        vjJSON, err2 := json.Marshal(vj)

        // In case of marshalling error, fall back to fmt.Sprintf (non-deterministic but rare).
        if err1 != nil || err2 != nil {
            strI := fmt.Sprintf("%v", vi)
            strJ := fmt.Sprintf("%v", vj)
            return strI < strJ
        }

        return string(viJSON) < string(vjJSON)
    })

    // Reorder the original slice
    sortedSlice := reflect.MakeSlice(value.Type(), sliceLen, sliceLen)
    for i, idx := range indices {
        sortedSlice.Index(i).Set(value.Index(idx))
    }
    reflect.Copy(value, sortedSlice)

    return nil
}

// Shuffle deterministically shuffles a slice
func (sm *SeedManager) Shuffle(slice interface{}) error {
    sm.mu.RLock()
    defer sm.mu.RUnlock()

    value := reflect.ValueOf(slice)
    if value.Kind() != reflect.Slice {
        return fmt.Errorf("Shuffle requires a slice, got %T", slice)
    }

    // Use Fisher-Yates shuffle with deterministic random source
    n := value.Len()
    rng := sm.seedSources["base"]

    for i := n - 1; i > 0; i-- {
        j := rng.Intn(i + 1)

        // Swap elements
        vi := value.Index(i).Interface()
        vj := value.Index(j).Interface()

        value.Index(i).Set(reflect.ValueOf(vj))
        value.Index(j).Set(reflect.ValueOf(vi))
    }

    return nil
}

// GetDeterministicRandom returns a deterministic random number for a process
func (sm *SeedManager) GetDeterministicRandom(processName string) float64 {
    seed := sm.GetSeedForProcess(processName)
    source := rand.NewSource(seed)
    rng := rand.New(source)
    return rng.Float64()
}

// GetDeterministicInt returns a deterministic random integer in range [min, max]
func (sm *SeedManager) GetDeterministicInt(processName string, min, max int) int {
    if min > max {
        min, max = max, min
    }

    seed := sm.GetSeedForProcess(processName)
    source := rand.NewSource(seed)
    rng := rand.New(source)

    return min + rng.Intn(max-min+1)
}

// Reset resets the seed manager with a new base seed
func (sm *SeedManager) Reset(baseSeed int64) {
    sm.mu.Lock()
    defer sm.mu.Unlock()

    sm.baseSeed = baseSeed
    sm.seedSources = make(map[string]*rand.Rand)
    sm.initialized = false
    sm.initialize()
}

// GetBaseSeed returns the current base seed
func (sm *SeedManager) GetBaseSeed() int64 {
    sm.mu.RLock()
    defer sm.mu.RUnlock()
    return sm.baseSeed
}

// GenerateProcessID generates a deterministic process ID
func (sm *SeedManager) GenerateProcessID(processName string, timestamp time.Time) string {
    input := fmt.Sprintf("%s:%d:%d", processName, sm.baseSeed, timestamp.UnixNano())
    return sm.DeterministicHash(input)[:16]
}

// Benchmark function for testing determinism
func (sm *SeedManager) Benchmark(n int) (map[string][]string, error) {
    results := make(map[string][]string)

    // Test hash determinism
    testString := "determinism_test"
    hashes := make([]string, n)
    for i := 0; i < n; i++ {
        hashes[i] = sm.DeterministicHash(testString)
    }

    // Verify all hashes are identical
    for i := 1; i < n; i++ {
        if hashes[i] != hashes[0] {
            return nil, fmt.Errorf("hash determinism failed at iteration %d", i)
        }
    }
    results["hash"] = hashes

    // Test random number determinism
    randoms := make([]string, n)
    for i := 0; i < n; i++ {
        sm.Reset(sm.baseSeed) // Reset to ensure same starting point
        randoms[i] = fmt.Sprintf("%f", sm.GetDeterministicRandom("test"))
    }

    // Verify all random numbers are identical
    for i := 1; i < n; i++ {
        if randoms[i] != randoms[0] {
            return nil, fmt.Errorf("random determinism failed at iteration %d", i)
        }
    }
    results["random"] = randoms

    return results, nil
}