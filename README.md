# 🔄 SyncMate 🌍

SyncMate is a tool for incrementally transfer the World of Code dataset to another machine (even in another continent). It uses Cloudflare R2 as the fast, affordable alternative to the shockingly expensive internet acceleration services in China. It is designed to mount the delta of the dataset to virtual filesystems, upload simultaneously, and download files in parallel. The global state is tracked in a Cloudflare D1 database to allow resumable uploads and downloads.

## Prerequisites

System requirements:
- Any x86_64/arm64 Linux distribution.
- GLibc 2.31 or later. That means you need at least Ubuntu 20.04, Debian 11, RHEL 9, or Fedora 32.
- Plenty of extra storage space plus the space for the WoC dataset itself on the destination clusters. I mean really plenty, like 5 TB.

### Setting up Cloudflare R2 and D1

1. **Create a Cloudflare account**: Sign up at [Cloudflare](https://www.cloudflare.com/).
2. **Create a Cloudflare R2 bucket**: Follow the [Cloudflare R2 documentation](https://developers.cloudflare.com/r2/) to create a bucket. Generate a S3-compatible keypair ([instructions](https://developers.cloudflare.com/r2/api/tokens/)) and note down your access key, secret key, and bucket name.
3. **Create a Cloudflare D1 database**: Follow the [Cloudflare D1 documentation](https://developers.cloudflare.com/d1/) to create a database. Now you have the database ID like aabbccff-1234-5678-90ab-cdef12345678.
4. **Generate an API token**: Create an API token with write permissions to your D1 database by following the [Cloudflare API token documentation](https://developers.cloudflare.com/api/tokens/). This token will be used to authenticate your requests.
5. **Create your `config.json`**: Create a configuration file with your Cloudflare credentials:

```json
{
    "access_key": "your_access_key",
    "secret_key": "your_secret_key",
    "account_id": "your_cloudflare_account_id",
    "database_id": "your_d1_database_id",
    "bucket": "your_bucket_name",
    "api_token": "your_cloudflare_api_token"
}
```

### Setting up WoC Profiles

1. **Install python-woc if you haven't already**: Follow the [python-woc installation instructions](https://github.com/ssc-oscar/python-woc).
2. **Create WoC profiles**: Use the following command to generate profiles for your source and destination datasets:

```bash
# on the source cluster
python3 -m woc.detect --with-digest --output woc.src.json --path /path/to/source
# on the destination cluster
python3 -m woc.detect --with-digest --output woc.dst.json --path /path/to/destination
```

### Setting up SyncMate

1. **Install Fuse**: SyncMate requires FUSE to mount the OffsetFS virtual filesystem. Install it using your package manager:

```bash
# For Debian/Ubuntu
sudo apt-get install fuse
# For Fedora/RHEL
sudo dnf install fuse
```

2. **Install SyncMate**: You can either download the pre-built binary:

```bash
wget https://github.com/hrz6976/syncmate/releases/download/latest/syncmate-linux-amd64 -O syncmate
# Or if you have fuse3 installed, but not fuse, use:
wget https://github.com/hrz6976/syncmate/releases/download/latest/syncmate-linux-amd64-fuse3 -O syncmate
chmod +x syncmate
```

Or build from source:

```bash
git clone https://github.com/hrz6976/syncmate.git
cd syncmate
go build -v -tags=fuse3 -ldflags="-s -w -X 'main.BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")' -X 'main.COMMIT_HASH=$(git rev-parse HEAD | cut -c1-7)'"
```

3. **Move everything to a directory**:

```bash
mkdir -p ~/syncmate/
mv syncmate ~/syncmate/
mv /path/to/woc.src.json ~/syncmate/
mv /path/to/woc.dst.json ~/syncmate/
mv /path/to/config.json ~/syncmate/
```

## Upload Files

I suggest using `screen` to run the upload command in the background, so you can monitor it later or check its progress in the log file.

```bash
screen -L -Logfile syncmate.log -S syncmate ./syncmate send -vv
```

## Download Files

It is pretty much like uploading, but you need to specify a cache directory and a destination directory. The cache directory is where SyncMate will store temporary files (can be terabytes large) during the download process, and the destination directory is where the new files will be placed. Files whose old versions are already on the destination will be placed in where they were before.

```bash
screen -L -Logfile syncmate.log -S syncmate ./syncmate recv -C /path/to/cache -D /path/to/destination -vv
```

## Commands

### `syncmate send`

Upload files to S3-compatible storage.

**Usage:**
```bash
syncmate send [flags]
```

**Flags:**
- `-s, --src`: WoC profile of the transfer source (default: "woc.src.json")
- `-d, --dst`: WoC profile of the transfer destination (default: "woc.dst.json") 
- `-c, --config`: Path to the configuration file (default: "config.json")
- `--skip-db`: Skip database operations

**Example:**
```bash
syncmate send --src woc.src.json --dst woc.dst.json --config config.json
```

### `syncmate recv`

Receive files from S3-compatible storage.

**Usage:**
```bash
syncmate recv [flags]
```

**Flags:**
- `-s, --src`: WoC profile of the transfer source (default: "woc.src.json")
- `-d, --dst`: WoC profile of the transfer destination (default: "woc.dst.json")
- `-c, --config`: Path to the configuration file (default: "config.json")
- `-C, --cache-dir`: Path to the cache directory
- `-D, --dest-dir`: Default destination directory for downloaded files (uses cache-dir if not specified)
- `--skip-db`: Skip database operations (useful for testing)
- `--delete-remote`: Delete files on remote after download (default: true)

**Example:**
```bash
syncmate recv --src woc.src.json --dst woc.dst.json --config config.json --cache-dir /tmp/cache
```

### `syncmate mount`

Mount the OffsetFS file system.

**Usage:**
```bash
syncmate mount [mountpoint] [flags]
```

**Flags:**
- `-c, --config`: Path to JSONL configuration file (required)
- `-d, --debug`: Enable debug output
- `-a, --allow-other`: Allow other users to access the filesystem
- `-r, --readonly`: Mount the filesystem in read-only mode

**Example:**
```bash
syncmate mount /mnt/offsetfs --config offsetfs.jsonl
```

### `syncmate taskgen`

Generate tasks for WoC transfer based on source and destination profiles.

**Usage:**
```bash
syncmate taskgen [flags]
```

**Flags:**
- `-s, --src`: WoC profile of the transfer source (default: "woc.src.json")
- `-d, --dst`: WoC profile of the transfer destination (default: "woc.dst.json")
- `-o, --output`: Output file for the generated tasks
- `--local-only`: Generate tasks for local files only, ignoring nonexisting files

**Example:**
```bash
syncmate taskgen --src woc.src.json --dst woc.dst.json --output tasks.jsonl
```

## Global Flags

- `-v, --verbose`: Verbose output (use -v, -vv, or --verbose=N for different levels)