package version

import (
    "github.com/Masterminds/semver/v3"
    "github.com/gin-gonic/gin"
)

// FeatureFlag represents a feature that may be enabled/disabled based on version
type FeatureFlag string

const (
    FeatureAdvancedSearch FeatureFlag = "advanced_search"
    FeatureBulkOperations FeatureFlag = "bulk_operations"
    FeatureRealTimeSync   FeatureFlag = "real_time_sync"
)

// FeatureVersions maps features to their minimum required version
var FeatureVersions = map[FeatureFlag]*semver.Version{
    FeatureAdvancedSearch:  semver.MustParse("1.1.0"),
    FeatureBulkOperations:  semver.MustParse("2.0.0"),
    FeatureRealTimeSync:    semver.MustParse("2.0.0"),
}

// IsFeatureEnabled checks if a feature is enabled for the current API version
func IsFeatureEnabled(c *gin.Context, feature FeatureFlag) bool {
    version, exists := c.Get("api_version")
    if !exists {
        return false
    }

    currentVersion, err := semver.NewVersion(string(version.(string)))
    if err != nil {
        return false
    }

    requiredVersion, exists := FeatureVersions[feature]
    if !exists {
        return false
    }

    return currentVersion.GreaterThanOrEqual(requiredVersion)
}

// VersionConstraint ensures a handler is only available for specific versions
func VersionConstraint(minVersion, maxVersion string) gin.HandlerFunc {
    return func(c *gin.Context) {
        version, exists := c.Get("api_version")
        if !exists {
            c.JSON(400, gin.H{"error": "API version not specified"})
            c.Abort()
            return
        }

        currentVersion, err := semver.NewVersion(string(version.(string)))
        if err != nil {
            c.JSON(400, gin.H{"error": "Invalid API version"})
            c.Abort()
            return
        }

        constraints, err := semver.NewConstraint(">= " + minVersion + ", <= " + maxVersion)
        if err != nil {
            c.JSON(500, gin.H{"error": "Invalid version constraint"})
            c.Abort()
            return
        }

        if !constraints.Check(currentVersion) {
            c.JSON(400, gin.H{"error": "API version not supported"})
            c.Abort()
            return
        }

        c.Next()
    }
} 