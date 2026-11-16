module.exports = {
  extends: ['github>OctopusDeploy/team-bmbb-renovate-config:default.json5'],

  // Produce conventional-commit PR titles so validate-pr-title passes
  semanticCommits: 'enabled',

  enabledManagers: ['gomod', 'github-actions'],

  // CI fails if go.mod and go.sum aren't tidy
  postUpdateOptions: ['gomodTidy'],

  packageRules: [
    {
      // OpenFeature SDK updates are worth releasing as a minor version, even if not breaking
      matchPackageNames: ['github.com/open-feature/go-sdk'],
      matchUpdateTypes: ['minor', 'patch'],
      semanticCommitType: 'feat',
    },
  ],
};
