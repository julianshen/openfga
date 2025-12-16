package valkey

import (
	"fmt"
)

const (
	storePrefix     = "stores"
	modelPrefix     = "models"
	assertionPrefix = "assertions"
	tuplePrefix     = "tuples"
	changelogPrefix = "changelog"
)

func storeKey(id string) string {
	return fmt.Sprintf("%s:%s", storePrefix, id)
}

func storesSetKey() string {
	return storePrefix
}

// storesByNameKey returns the key for the Set of store IDs with a given name
func storesByNameKey(name string) string {
	return fmt.Sprintf("stores:by_name:%s", name)
}

func authorizationModelKey(storeID, modelID string) string {
	return fmt.Sprintf("%s:%s:%s", modelPrefix, storeID, modelID)
}

func latestAuthorizationModelKey(storeID string) string {
	return fmt.Sprintf("%s:%s:latest", modelPrefix, storeID)
}

func assertionsKey(storeID, modelID string) string {
	return fmt.Sprintf("%s:%s:%s", assertionPrefix, storeID, modelID)
}

func changelogKey(storeID string) string {
	return fmt.Sprintf("%s:%s", changelogPrefix, storeID)
}

// Tuple keys
// tuples:{store_id}:{object}:{relation}:{user} -> ""
func tupleKey(storeID, object, relation, user string) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s", tuplePrefix, storeID, object, relation, user)
}

// Index keys
// index:obj_rel:{store_id}:{object}:{relation} -> Set of user
func indexObjectRelationKey(storeID, object, relation string) string {
	return fmt.Sprintf("index:obj_rel:%s:%s:%s", storeID, object, relation)
}

// index:user:{store_id}:{user} -> Set of object#relation
func indexUserKey(storeID, user string) string {
	return fmt.Sprintf("index:user:%s:%s", storeID, user)
}

// index:userset:{store_id}:{object}:{relation} -> Set of user (generic)
// This is to optimize ReadUsersetTuples
func indexUsersetKey(storeID, object, relation string) string {
	return fmt.Sprintf("index:userset:%s:%s:%s", storeID, object, relation)
}
