package determinism_test

import (
    "testing"
    "time"

    "semantic-analyzer/pkg/determinism"
)

func TestSeedManager_Determinism(t *testing.T) {
    sm := determinism.NewSeedManager(42)
    
    // Test hash determinism
    hash1 := sm.DeterministicHash("test")
    hash2 := sm.DeterministicHash("test")
    
    if hash1 != hash2 {
        t.Errorf("Hash not deterministic: %s != %s", hash1, hash2)
    }
    
    // Test 100 iterations
    for i := 0; i < 100; i++ {
        currentHash := sm.DeterministicHash("test")
        if currentHash != hash1 {
            t.Errorf("Hash changed at iteration %d: %s != %s", i, currentHash, hash1)
        }
    }
}

func TestSeedManager_GetSeedForProcess(t *testing.T) {
    sm := determinism.NewSeedManager(42)
    
    seed1 := sm.GetSeedForProcess("process1")
    seed2 := sm.GetSeedForProcess("process1")
    
    if seed1 != seed2 {
        t.Errorf("Seed not deterministic for same process: %d != %d", seed1, seed2)
    }
    
    seed3 := sm.GetSeedForProcess("process2")
    if seed1 == seed3 {
        t.Errorf("Different processes should have different seeds: %d == %d", seed1, seed3)
    }
}

func TestSeedManager_StableSort(t *testing.T) {
    sm := determinism.NewSeedManager(42)
    
    // Test with strings
    strings := []string{"z", "a", "m", "c", "b"}
    expected := []string{"a", "b", "c", "m", "z"}
    
    err := sm.StableSort(strings)
    if err != nil {
        t.Fatalf("StableSort failed: %v", err)
    }
    
    for i, s := range strings {
        if s != expected[i] {
            t.Errorf("Sort failed at index %d: got %s, want %s", i, s, expected[i])
        }
    }
    
    // Test determinism across runs
    strings1 := []string{"z", "a", "m", "c", "b"}
    strings2 := []string{"z", "a", "m", "c", "b"}
    
    sm1 := determinism.NewSeedManager(42)
    sm2 := determinism.NewSeedManager(42)
    
    sm1.StableSort(strings1)
    sm2.StableSort(strings2)
    
    for i := range strings1 {
        if strings1[i] != strings2[i] {
            t.Errorf("Sort not deterministic at index %d: %s != %s", i, strings1[i], strings2[i])
        }
    }
}

func TestSeedManager_Benchmark(t *testing.T) {
    sm := determinism.NewSeedManager(42)
    
    results, err := sm.Benchmark(10)
    if err != nil {
        t.Fatalf("Benchmark failed: %v", err)
    }
    
    if len(results["hash"]) != 10 {
        t.Errorf("Expected 10 hash results, got %d", len(results["hash"]))
    }
    
    // Verify all hashes are identical
    hashes := results["hash"].([]string)
    for i := 1; i < len(hashes); i++ {
        if hashes[i] != hashes[0] {
            t.Errorf("Hash not deterministic at iteration %d", i)
        }
    }
}

func TestSeedManager_GenerateProcessID(t *testing.T) {
    sm := determinism.NewSeedManager(42)
    timestamp := time.Now()
    
    id1 := sm.GenerateProcessID("test", timestamp)
    id2 := sm.GenerateProcessID("test", timestamp)
    
    if id1 != id2 {
        t.Errorf("Process ID not deterministic: %s != %s", id1, id2)
    }
    
    // Different process name should give different ID
    id3 := sm.GenerateProcessID("test2", timestamp)
    if id1 == id3 {
        t.Errorf("Different process names should give different IDs")
    }
}