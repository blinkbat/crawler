package core

import (
	"reflect"
	"strings"
	"testing"
)

// TestEnemyProbabilityFieldsBounded reflect-walks EnemyDefinition for probability
// fields (float64 named *Chance / *Percent) and asserts (1) each is covered by
// enemyStatBounds so the init validator bounds-checks it, and (2) every enemy's value
// is within [0,1]. Guards the drift that let LifestealPercent ship unbounded: a new
// proc field is caught here without anyone remembering to extend the manual bounds
// literal or the validator.
func TestEnemyProbabilityFieldsBounded(t *testing.T) {
	isProb := func(name string) bool {
		return strings.HasSuffix(name, "Chance") || strings.HasSuffix(name, "Percent")
	}
	defT := reflect.TypeOf(EnemyDefinition{})
	boundsT := reflect.TypeOf(enemyStatBounds{})
	for i := 0; i < defT.NumField(); i++ {
		f := defT.Field(i)
		if f.Type.Kind() != reflect.Float64 || !isProb(f.Name) {
			continue
		}
		if _, ok := boundsT.FieldByName(f.Name); !ok {
			t.Errorf("EnemyDefinition.%s is a probability field but not covered by enemyStatBounds — add it so the init validator bounds-checks it", f.Name)
		}
	}
	for _, def := range enemyDefinitions {
		v := reflect.ValueOf(def)
		for i := 0; i < defT.NumField(); i++ {
			f := defT.Field(i)
			if f.Type.Kind() != reflect.Float64 || !isProb(f.Name) {
				continue
			}
			if got := v.Field(i).Float(); got < 0 || got > 1 {
				t.Errorf("enemy %q field %s = %v outside [0,1]", def.Name, f.Name, got)
			}
		}
	}
}
