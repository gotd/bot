# bot

Bot for gotd chats based on [gotd/td](https://github.com/gotd/td).

## Goals

This bot is like a canary node: platform for experiments, testing
and ensuring stability.

## Commands

* `/bot` - answers "What?"
* `/json` - inspects replied message
* `/dice` - sends dice
* `/stat` - prints metrics

## Skip deploy

Add `!skip` to commit message.

## Migrations

### Add migration

To add migration named `some-migration-name`:

```console
atlas migrate --env dev diff some-migration-name
```

## Golden files

In package directory:

```console
go test -update
```
