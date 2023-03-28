# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.5.0] - 2023-03-28

### Added

- Add acceptance tests.
- Add transit gateway share reconciler.

## [1.4.4] - 2023-03-15

### Changed

- Change client used to create/delete routes.

## [1.4.3] - 2023-03-15

### Added

- Added the use of the runtime/default seccomp profile

## [1.4.2] - 2023-02-16

### Fixed

- Use short tag name for TGW attachment subnets.

## [1.4.1] - 2023-01-27

### Changed

- Change subnet selection condition for transit gateway attachment.
- Add default vaulue for `serviceType`.

## [1.4.0] - 2023-01-19

### Changed

- Only send SNS message when the Transit Gateway attachment is in pending approval state.
- Add cluster name as tag `Name` when creating the Transit Gateway attachment.

### Fixed

- Fix deletion when the AWSCluster is still around but the finalizer has been removed.

## [1.3.0] - 2023-01-17

### Added

- Use annotations from k8smetadata package.

### Fixed

- Fix pushing image to AliYun.
- Use proper date format in logs.

## [1.2.4] - 2023-01-03

### Fixed

- Fix deletion issue when the AWSCluster is already gone but finalizer was not removed.
- Fix attaching VPC to transit gateway if workload cluster is in another AWS account

## [1.2.3] - 2022-11-29

### Fixed

- Add missing RBAC permissions to create k8s events.

## [1.2.2] - 2022-11-25

### Fixed

- Populated "Postfach" in SNS message

## [1.2.1] - 2022-11-17

### Fixed

- Fix incorrectly skipping adding prefix list to route tables when one of the route tables already has it but others don't

## [1.2.0] - 2022-10-21

### Added

- UserManaged mode support

## [1.1.4] - 2022-09-26

### Fixed

- updating the status condition to "true" when reconcile finished

## [1.1.3] - 2022-09-23

### Fixed

- Nil pointer check

## [1.1.2] - 2022-09-23

### Fixed

- Correctly check for existing routes matching

## [1.1.1] - 2022-09-23

### Fixed

- Check for CIDR before attempting to remove from prefix list
- Added missing `/status` subresource permission to RBAC role

## [1.1.0] - 2022-09-23

### Added

- Status condition to cluster

### Fixed

- Add finalizer to AWSCluster resource too

## [1.0.2] - 2022-09-23

### Fixed

- Wait for new VPC to be available

## [1.0.1] - 2022-09-23

### Fixed

- Don't attempt to add same route twice

## [1.0.0] - 2022-09-23

### Fixed

- Reduced max entries for prefix list

### Changed

- Refactored logging for cleaner output and less noise

### Added

- Support getting prefix list based on ID in annotation
- Check for conflicting CIDR when adding to prefix list

## [0.2.3] - 2022-09-22

## [0.2.2] - 2022-09-22

### Fixed

- Check prefix list entries prior to adding new entry

## [0.2.1] - 2022-09-22

### Fixed

- Pass version when modifying prefix list

## [0.2.0] - 2022-09-22

### Added

- Create Prefix List for use with the TGW routing
- Added `network-topology.giantswarm.io/prefix-list` annotation support

### Fixed

- Add TGW route to cluster subnet route tables

## [0.1.7] - 2022-09-20

### Fixed

- Use correct value for TGW attatchment tags

## [0.1.6] - 2022-09-20

### Added

- Check TGW state before attempting to create attachment

### Fixed

- Use only one subnet per AZ for TGW attachment

## [0.1.5] - 2022-09-16

### Fixed

- Typo in rbac resource

## [0.1.4] - 2022-09-16

### Fixed

- Initialise AWS client after controller has been started (see https://github.com/kubernetes-sigs/controller-runtime/issues/607)

## [0.1.3] - 2022-09-16

### Fixed

- Added missing scheme registration for capa

## [0.1.2] - 2022-09-16

### Fixed

- Use the IAM role from the MCs AWSClusterRoleIdentity

## [0.1.1] - 2022-09-16

### Fixed

- Typo in leader-elect argument

## [0.1.0] - 2022-09-16

### Added

- AWS credentials secret

## [0.0.5] - 2022-09-15

### Fixed

- Indentation

## [0.0.4] - 2022-09-15

### Fixed

- Correct CircleCI tasks

## [0.0.3] - 2022-09-15

### Fixed

- Removed incorrect GCP mentions

## [0.0.2] - 2022-09-15

### Changed

* Refactored / cleaned up some code

### Added

* Tests for checking paused cluster state
* Test for checking registrars get called
* Test for cluster not existing
* Tests to cover creation of TGW and attachments

## [0.0.1] - 2022-09-06

### Added

- Helm chart

[Unreleased]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.5.0...HEAD
[1.5.0]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.4.4...v1.5.0
[1.4.4]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.4.3...v1.4.4
[1.4.3]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.4.2...v1.4.3
[1.4.2]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.4.1...v1.4.2
[1.4.1]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.4.0...v1.4.1
[1.4.0]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.3.0...v1.4.0
[1.3.0]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.2.4...v1.3.0
[1.2.4]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.2.3...v1.2.4
[1.2.3]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.2.2...v1.2.3
[1.2.2]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.2.1...v1.2.2
[1.2.1]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.2.0...v1.2.1
[1.2.0]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.1.4...v1.2.0
[1.1.4]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.1.3...v1.1.4
[1.1.3]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.1.2...v1.1.3
[1.1.2]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.1.1...v1.1.2
[1.1.1]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.0.2...v1.1.0
[1.0.2]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/giantswarm/aws-network-topology-operator/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.2.3...v1.0.0
[0.2.3]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.1.7...v0.2.0
[0.1.7]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.0.5...v0.1.0
[0.0.5]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.0.4...v0.0.5
[0.0.4]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/giantswarm/aws-network-topology-operator/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/giantswarm/aws-network-topology-operator/releases/tag/v0.0.1
