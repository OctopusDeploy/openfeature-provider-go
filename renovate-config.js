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
  ],

  packageRules: [
    {
      // OpenFeature SDK updates are worth releasing as a minor version, even if not breaking
      matchPackageNames: ['github.com/open-feature/go-sdk'],
      matchUpdateTypes: ['minor', 'patch'],
      semanticCommitType: 'feat',
    },
  ],
};
