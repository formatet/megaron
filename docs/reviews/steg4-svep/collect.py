#!/usr/bin/env python3
"""Pinned read-only external measurements; caches stay under the user's home."""
import concurrent.futures, datetime, json, os, pathlib, subprocess, time
ROOT = pathlib.Path(__file__).resolve().parents[3]
OUT = pathlib.Path(__file__).resolve().parent / "raw"
BASE = "f586270d40840fc8afb550bca80a2ee73d4e7a97"
CACHE = pathlib.Path.home() / ".cache/megaron-steg4"
ENV = {k: os.environ[k] for k in ("HOME", "PATH", "USER", "LANG") if k in os.environ}
ENV.update(GOCACHE=str(CACHE / "gocache"), GOMODCACHE=str(CACHE / "gomodcache"),
           GOTMPDIR=str(CACHE / "gotmp"), TMPDIR=str(CACHE / "tmp"),
           npm_config_cache=str(CACHE / "npm-cache"), CGO_ENABLED="1", GOOS="linux", GOARCH="amd64")
for directory in (OUT, CACHE / "gotmp", CACHE / "tmp"):
    directory.mkdir(parents=True, exist_ok=True)
subprocess.run(["git", "-C", str(ROOT), "merge-base", "--is-ancestor", BASE, "HEAD"], check=True)
# Do not silently measure modified sources. Measurement files may be untracked.
assert not subprocess.check_output(["git", "-C", str(ROOT), "diff", BASE, "--", "server", "web"], text=True)
def run(name, argv, cwd=ROOT, data=None):
    started = time.monotonic()
    with (OUT / (name + ".stdout")).open("w") as out, (OUT / (name + ".stderr")).open("w") as err:
        result = subprocess.run(argv, cwd=cwd, env=ENV, input=data, text=True, stdout=out, stderr=err)
    record = dict(command=argv, cwd=str(cwd.relative_to(ROOT)), exit=result.returncode,
                  elapsed_seconds=round(time.monotonic()-started, 3))
    (OUT / (name + ".run.json")).write_text(json.dumps(record, indent=2)+"\n")
    print(name, record["exit"], record["elapsed_seconds"], flush=True)
    return record
files = sorted(str(p.relative_to(ROOT)) for p in (ROOT / "server").rglob("*.go"))
(OUT / "go-files.txt").write_text("\n".join(files)+"\n")
prod = [f for f in files if not f.endswith("_test.go")]
(OUT / "go-production-files.txt").write_text("\n".join(prod)+"\n")
meta = dict(base=BASE, measured_at=datetime.datetime.now().astimezone().isoformat(),
            cache=str(CACHE), platform="linux/amd64", build_tags=[],
            versions=dict(dupl="github.com/mibk/dupl@v1.1.0", deadcode="golang.org/x/tools/cmd/deadcode@v0.51.0", jscpd="5.4.0"),
            go=subprocess.check_output(["go", "version"], text=True).strip())
(OUT / "provenance.json").write_text(json.dumps(meta,indent=2)+"\n")
jscpd = str(CACHE / "tools/node_modules/.bin/jscpd")
dupl = str(CACHE / "bin/dupl")
deadcode = str(CACHE / "bin/deadcode")
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
    jobs = [pool.submit(run,"dupl-all",[dupl,"-plumbing","-t","100","-files"],ROOT,"\n".join(files)+"\n"),
            pool.submit(run,"dupl-production",[dupl,"-plumbing","-t","100","-files"],ROOT,"\n".join(prod)+"\n"),
            pool.submit(run,"dupl-production-50",[dupl,"-plumbing","-t","50","-files"],ROOT,"\n".join(prod)+"\n"),
            pool.submit(run,"jscpd",[jscpd,"web/static/js","--format","javascript","--min-tokens","100","--min-lines","5","--mode","weak","--reporters","json","--output",str(OUT / "jscpd"),"--silent","--no-colors","--fail-on-empty"])]
    records=[job.result() for job in jobs]
# RTA arms are sequential to avoid overlapping their memory use.
records.append(run("deadcode-production",[deadcode,"-json","./..."],ROOT / "server"))
records.append(run("deadcode-with-tests",[deadcode,"-json","-test","./..."],ROOT / "server"))
(OUT / "runs.json").write_text(json.dumps(records,indent=2)+"\n")
assert all(r["exit"] == 0 for r in records), "inspect stderr; failed arm is not evidence"
