"""Test för alfa_funnel.py / alfa_db.py.

  python3 tools/alfa_funnel_test.py                       # bara de rena testerna
  ALFA_TEST_DSN=postgres://... python3 tools/alfa_funnel_test.py        # + mot en riktig DB
  ALFA_TEST_EXPECT=built,trained,marched,... (valfritt) kräver att de kolumnerna är fyllda
  ALFA_TEST_PSQL_CMD="docker exec -i alfa-pg psql -U poleia -d poleia" python3 tools/alfa_funnel_test.py

DB-testerna hoppas över utan ALFA_TEST_DSN / ALFA_TEST_PSQL_CMD. De förutsätter en värld där minst en
människa har grundat stad (acceptansrigg eller testvärld) och SKRIVER ALDRIG.
"""
import os
import sys
import unittest
from datetime import datetime, timedelta, timezone

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import alfa_db  # noqa: E402
import alfa_funnel as F  # noqa: E402

DSN = os.environ.get("ALFA_TEST_DSN")
PSQL = os.environ.get("ALFA_TEST_PSQL_CMD")


class CalendarTest(unittest.TestCase):
    def test_tick_lookup(self):
        t0 = datetime(2026, 1, 1, tzinfo=timezone.utc)
        pairs = [(t0 + timedelta(hours=i), i) for i in range(1, 11)]
        c = F.Calendar(pairs, current_tick=10)
        self.assertEqual(c.tick(t0 + timedelta(hours=5, minutes=30)), 5)
        self.assertEqual(c.tick(t0 + timedelta(hours=100)), 10)
        self.assertEqual(c.tick(t0 + timedelta(hours=-3)), 0)  # före äldsta eventet: extrapolation, aldrig negativt

    def test_nedtid_ger_inte_falsk_kadens(self):
        # tick 5 -> 6 med 10 timmars glapp: en tid mitt i glappet är fortfarande tick 5
        t0 = datetime(2026, 1, 1, tzinfo=timezone.utc)
        c = F.Calendar([(t0, 5), (t0 + timedelta(hours=10), 6)], current_tick=6)
        self.assertEqual(c.tick(t0 + timedelta(hours=7)), 5)


@unittest.skipUnless(DSN or PSQL, "ange ALFA_TEST_DSN eller ALFA_TEST_PSQL_CMD")
class DbTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.db = alfa_db.Db(dsn=DSN, psql_cmd=PSQL)

    def test_skrivskyddad(self):
        # En skrivning måste vägras av servern (BEGIN READ ONLY), inte bara av vår goodwill.
        with self.assertRaises(SystemExit) as cm:
            self.db.raw("UPDATE worlds SET name = name")
        self.assertIn("read-only", str(cm.exception).lower())

    def test_funnel_fyller_det_som_hänt(self):
        world = alfa_db.pick_world(self.db)
        data = F.load(self.db, world)
        rows, _ = F.build(data, world)
        self.assertTrue(rows, "ingen människa i världen")
        header = [h for _, h in F.COLUMNS]
        founders = [r for r in rows if r[1 + [k for k, _ in F.COLUMNS].index("founded")]]
        self.assertTrue(founders, "ingen har grundat stad — testvärlden är för tom")
        for r in rows:
            self.assertEqual(len(r), 1 + len(header) + 2)
            for cell in r[1:1 + len(header)]:
                if cell and cell != "nu":
                    n = int(cell.rstrip("~"))
                    self.assertTrue(0 <= n <= world["current_tick"] + 1, cell)

    def test_förväntade_kolumner_är_fyllda(self):
        # ALFA_TEST_EXPECT="built,trained,..." — kolumner som provspelet tog; minst en rad ska ha dem.
        # (Mutationsprovet: byt en källkonstant i alfa_funnel.py -> den här testen blir röd.)
        want = [k for k in os.environ.get("ALFA_TEST_EXPECT", "").split(",") if k]
        if not want:
            self.skipTest("ALFA_TEST_EXPECT ej satt")
        world = alfa_db.pick_world(self.db)
        rows, _ = F.build(F.load(self.db, world), world)
        keys = [k for k, _ in F.COLUMNS]
        for k in want:
            self.assertTrue(any(r[1 + keys.index(k)] for r in rows), "kolumnen %s är tom för alla" % k)

    def test_funnel_ordning(self):
        # grundad stad kommer före första bygget/rekryteringen (kausalitet) för alla som har dem
        world = alfa_db.pick_world(self.db)
        rows, _ = F.build(F.load(self.db, world), world)
        keys = [k for k, _ in F.COLUMNS]
        for r in rows:
            v = lambda k: int(r[1 + keys.index(k)].rstrip("~")) if r[1 + keys.index(k)] not in ("", "nu") else None
            if v("founded") is not None:
                for k in ("built", "trained", "marched"):
                    if v(k) is not None:
                        self.assertGreaterEqual(v(k), v("founded"), "%s före grundandet för %s" % (k, r[0]))


if __name__ == "__main__":
    unittest.main()
