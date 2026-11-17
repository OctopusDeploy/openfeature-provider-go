module.exports = {
  extends: ['github>OctopusDeploy/team-bmbb-renovate-config:default.json5'],

  // Produce conventional-commit PR titles so validate-pr-title passes
  semanticCommits: 'enabled',

  enabledManagers: ['gomod', 'github-actions', 'custom.regex'],

  // CI fails if go.mod and go.sum aren't tidy
  postUpdateOptions: ['gomodTidy'],

  customManagers: [
    {
      // The github-actions manager can't read go-version from a matrix, so track the latest entry the same way it
      // tracks setup-go. The floor is left out because it moves by hand with go.mod.
      customType: 'regex',
      managerFilePatterns: ['/^\\.github/workflows/main\\.yml$/'],
      matchStrings: ['label: latest\\s+go-version: "(?<currentValue>[^"]+)"'],
      depNameTemplate: 'go',
      datasourceTemplate: 'github-releases',
      packageNameTemplate: 'actions/go-versions',
      versioningTemplate: 'npm',
      extractVersionTemplate: '^(?<version>\\d+\\.\\d+\\.\\d+)(-\\d+)?$',
    },
    {
      // govulncheck runs through go run, which no built-in manager reads
      customType: 'regex',
      managerFilePatterns: ['/^\\.github/workflows/main\\.yml$/'],
      matchStrings: ['golang\\.org/x/vuln/cmd/govulncheck@(?<currentValue>v[^\\s]+)'],
      depNameTemplate: 'golang.org/x/vuln',
      datasourceTemplate: 'go',
    },
  ],

  packageRules: [
    {
      // Keep the CI Go versions in one PR so lint and the latest test entry can't drift apart. The datasource
      // leaves out go.mod's go line, which uses golang-version.
      matchDepNames: ['go'],
      matchDatasources: ['github-releases'],
      groupName: 'CI Go version',
    },
    {
      // OpenFeature SDK updates are worth releasing as a minor version, even if not breaking
      matchPackageNames: ['github.com/open-feature/go-sdk'],
      matchUpdateTypes: ['minor', 'patch'],
      semanticCommitType: 'feat',
    },
  ],
};
