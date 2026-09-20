---
title: Terms of service
effective: 2026-09-20
status: draft for launch review
---

# Terms of service

These terms govern your use of repose, a service that runs a persistent
Linux environment for each of your projects on shared servers we operate,
reachable over SSH, where coding agents keep working after your laptop
closes. By creating an account you agree to them.

## The service

Each project gets one environment: a virtual machine of the size class you
choose, a disk of the size you choose, a network connection to the
internet, and the tools and agents we ship. Environments run until you
stop them. We build the environment's configuration from the description
you give us and apply it in place; when a change needs a reboot we ask
first. We snapshot every environment nightly and when you stop it, and you
can restore any snapshot we still hold.

We operate on infrastructure we rent; we will tell you in advance when we
move it and we will move your data with it.

## Your account

You sign in with a GitHub account. One person, one account; you are
responsible for what happens under it, including what your agents do. Keep
your laptop's SSH key and your CLI login private; if you lose a laptop,
`repose logout` from another device revokes its certificates, and every
certificate expires on its own within twelve hours.

## Coding agents and their providers

Coding agents such as Claude Code run inside your environment under your
own account with that agent's provider. repose does not hold, proxy, or
resell those credentials. You are responsible for complying with each
provider's terms for hosted use.

In particular: the platform never copies or stores your Claude Code login;
you authenticate inside the environment, and the only alternative we offer
is a token you generate yourself and store as a named secret of your own
project. The agent binaries we ship are the providers' own releases,
unmodified.

## Acceptable use

Your environment is yours to use for software development and the
workloads that come with it: builds, tests, containers, browsers, agents.
You may not use it to mine cryptocurrency, to scan or attack networks or
systems you do not own, to send unsolicited mail, to host content that is
illegal where we or you are, or to attempt to reach other tenants'
environments, our servers, or the cloud provider's metadata services. We
record process names and network volumes to notice these things; the
privacy policy says exactly what we record and what we never record.

Each project has a bandwidth ceiling, a monthly egress allowance, and
build limits. Accounts start with a limit on the number of projects and on
the largest size class until a first invoice is paid.

## Billing

You provide a payment card before your first environment starts. New
accounts receive a trial credit consumed at the same hourly rates.
Environments are billed by the hour while running, with a monthly cap per
project equal to that size class's flat price; disk is billed by the
gigabyte-month while the project exists; egress beyond the included
allowance is billed by the gigabyte. Invoices are monthly. If a payment
fails we retry; after three days we stop your environments, and after
thirty days we may delete them. Prices are published on the site and a
change takes effect at the start of your next billing month.

## Your data

Your environments, their disks and snapshots, and the secrets you store
are yours. We access them only to operate the service, to investigate an
incident or an abuse report, or at your request, and every such access is
logged. We may suspend an account that breaks these terms; suspension
stops environments and preserves data on the normal retention schedule.
Destroying a project deletes its disk at once and its last snapshot after
thirty days. Cancelling your account stops everything at once and deletes
all data after thirty days.

## Availability and liability

We aim for the service to be available and we tell you when it is not, but
we make no guarantee of uptime, and an agent that was running when a
server failed may need to be started again from a snapshot. To the extent
the law allows, our liability to you is limited to the fees you paid us in
the three months before the claim, and we are not liable for indirect or
consequential loss, including work an agent did not finish. Nothing here
limits liability that cannot be limited by law.

## Changes

We may change these terms; we publish changes with their effective date
and notify you by email at least fourteen days before a change that
reduces what you get or increases what you pay. Continuing to use the
service after that date is acceptance.

## Contact

Questions, notices and security reports go to the contact address
published on repose.herakraft.co.
