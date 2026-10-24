#!/usr/bin/env python3
import unittest
import day_guard as guard
class TimeLanguageTest(unittest.TestCase):
 def test_explicit_game_durations(self):
  for text in ('≈12 game days','~12 game days in real time','food for 12 game days','+20 per game day','a 30-game-day expedition','1 unchallenged game day','today 18:34','Thu 09:00','Day 6 of the Olive, Year 1','the Shadow Days of the Goddess'):
   self.assertFalse(guard.forbidden_player_time(text),text)
 def test_bare_lengths_and_rates(self):
  for text in ('food for 12 days','+20 per day','a 30-day expedition','day','days','three days','a day','in a day','for 2 Days','game days, then 3 days','Day 6 of the Olive, Year 1; food for 3 days'):
   self.assertTrue(guard.bare_days(text),text)
 def test_nested_templates_still_examined(self):
  self.assertIn('3 days',[v for _,v in guard.js_strings('const s=`${ok ? "3 days" : `4 game days`}`;')])
if __name__=='__main__':unittest.main()
