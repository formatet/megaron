import base64
import fcntl
import io
import json
from pathlib import Path
import tempfile
import time
import unittest
from unittest.mock import patch

from luna_command import LIMIT, atomic_json, expires_at, main, refresh_config, validate_args
import luna_fleet


def token(exp):
    payload = base64.urlsafe_b64encode(json.dumps({"exp": exp}).encode()).decode().rstrip("=")
    return "header." + payload + ".signature"


class LunaCommandTests(unittest.TestCase):
    def test_another_run_cannot_take_the_same_players_turn(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "hephaistos").mkdir()
            with (root / "hephaistos/round.lock").open("a") as held:
                fcntl.flock(held, fcntl.LOCK_EX | fcntl.LOCK_NB)
                with patch.object(luna_fleet, "ROOT", root), patch.object(luna_fleet, "_run_round") as play:
                    self.assertEqual(luna_fleet.run_round("hephaistos")["state"], "already_running")
                    play.assert_not_called()

    def test_round_cannot_use_another_profile_directory(self):
        with tempfile.TemporaryDirectory() as tmp:
            own = Path(tmp) / "own"
            other = Path(tmp) / "other/rounds/1"
            with patch.dict("os.environ", {"MEGARON_LUNA_ROUND": str(other)}):
                with self.assertRaisesRegex(SystemExit, "No active round"):
                    main(own)

    def test_command_budget_stops_before_another_cli_or_refresh(self):
        with tempfile.TemporaryDirectory() as tmp:
            own = Path(tmp) / "own"
            round_dir = own / "rounds/1"
            round_dir.mkdir(parents=True)
            (round_dir / "commands.jsonl").write_text("{}\n" * LIMIT)
            with patch.dict("os.environ", {"MEGARON_LUNA_ROUND": str(round_dir)}):
                with patch("luna_command.subprocess.run") as run_cli:
                    with self.assertRaisesRegex(SystemExit, "command limit"):
                        main(own)
                    run_cli.assert_not_called()

    def test_first_time_player_can_open_root_help(self):
        self.assertEqual(validate_args(["--help"]), ["help"])
        self.assertEqual(validate_args(["-j", "--help"]), ["help"])

    def test_player_can_explore_reassign_and_address_build_hex(self):
        for args in (["place", "grain", "5", "-1"],
                     ["build", "--type", "mine", "--hex", "3,4"],
                     ["unit", "march", "--unit", "uuid", "--q", "3", "--r", "4", "--intent", "explore", "--ticks", "12"],
                     ["trade-offer", "--to", "Known City", "--want-good", "tin", "--want-qty", "1", "--offer-silver", "2"]):
            self.assertEqual(validate_args(args), args)

    def test_account_admin_and_connection_overrides_are_rejected(self):
        for args in (["login"], ["god"], ["reports"], ["password"], ["watch"],
                     ["status", "--token=secret"], ["status", "--server", "other"],
                     ["unit", "execute-shell"], ["sh", "-c", "anything"]):
            with self.assertRaises(ValueError):
                validate_args(args)

    def test_valid_token_does_not_refresh_every_command(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "config.json"
            cfg = {"token": token(time.time() + 3600)}
            atomic_json(path, cfg)
            self.assertEqual(refresh_config(path, opener=lambda *_: self.fail("unnecessary refresh")), cfg)

    def test_refresh_rotates_both_tokens_and_preserves_identity(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "config.json"
            cfg = {"server": "http://example.invalid", "world_id": "world", "player_id": "own-player",
                   "token": token(time.time() - 5), "refresh_token": "old-refresh", "password": "unchanged"}
            atomic_json(path, cfg)
            def opener(req, timeout):
                self.assertEqual(req.full_url, "http://example.invalid/api/v1/auth/refresh")
                self.assertEqual(json.loads(req.data), {"refresh_token": "old-refresh"})
                return io.StringIO(json.dumps({"access_token": token(time.time() + 86400), "refresh_token": "rotated"}))
            result = refresh_config(path, opener=opener)
            self.assertEqual(result["player_id"], "own-player")
            self.assertEqual(result["password"], "unchanged")
            self.assertEqual(result["refresh_token"], "rotated")
            self.assertGreater(expires_at(result["token"]), time.time() + 80000)
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)

    def test_bad_refresh_preserves_original_credentials(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "config.json"
            cfg = {"server": "http://example.invalid", "token": token(1), "refresh_token": "original"}
            atomic_json(path, cfg)
            before = path.read_bytes()
            with self.assertRaisesRegex(RuntimeError, "token renewal failed"):
                refresh_config(path, opener=lambda *_args, **_kwargs: io.StringIO("{}"))
            self.assertEqual(path.read_bytes(), before)


if __name__ == "__main__":
    unittest.main()
