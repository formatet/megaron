#!/usr/bin/env python3
"""Naming failures must not block movement or healthy journeys."""
import pathlib
import subprocess
ROOT=pathlib.Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/flod-doparratt/massif'
SOURCE=ROOT/'server/internal/combat/place_reach.go'
original=SOURCE.read_bytes()
def mutation(label,old,new,test,expected):
    try:
        assert original.decode().count(old)==1
        SOURCE.write_text(original.decode().replace(old,new,1))
        result=subprocess.run(['tools/gotest.sh','./internal/combat/','-run',test,'-v'],cwd=ROOT,text=True,capture_output=True)
        output=result.stdout+result.stderr
        (OUT/('mutation-'+label+'.log')).write_text(output)
        assert result.returncode and expected in output,output
        print(label+': named red')
    finally:
        SOURCE.write_bytes(original)
warn='slog.Warn("place reach before arrival failed; continuing movement", "world", e.WorldID, "error", err)'
mutation('arrival-block',warn,warn+'\n return err','TestPlaceReachArrivalFlushFailureStillRunsMovement','naming flush failure stopped movement')
warn='slog.Warn("place reach skips invalid unit route", "world", worldID, "unit", v.id)\n\t\t\tcontinue'
mutation('corrupt-global-failure',warn,warn.replace('continue','return nil, fmt.Errorf("broken journey")'),'TestPlaceReachCorruptJourneyDoesNotBlockHealthyArrival','corrupt journey blocked healthy arrival claim')
with (OUT/'restored-arrival.log').open('w') as log:
    subprocess.run(['tools/gotest.sh','./internal/combat/','-run','TestPlaceReach(ArrivalFlushFailure|CorruptJourney)','-v'],cwd=ROOT,stdout=log,stderr=subprocess.STDOUT,check=True)
print('arrival: restored green')
