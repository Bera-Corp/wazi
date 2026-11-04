import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock
import lifecycle_service as service
import lifecycle_rollout as rollout
from test_platform_lifecycle import fixture
import validate


class SchedulerTests(unittest.TestCase):
    def test_state_survives_ticks_without_reissuing_mutations(self):
        plan,_,_=fixture()
        with tempfile.TemporaryDirectory() as temporary:
            path=Path(temporary)/"rollouts.sqlite";db=rollout.connection(path)
            try:
                db.execute("INSERT INTO rollouts VALUES(?,?,?,?)",(plan["rolloutId"],validate.canonical_digest(plan),json.dumps(plan),json.dumps({"phase":"forward","wave":0,"target":0,"action":0,"completedTargets":[]})));db.commit()
            finally:db.close()
            def request(target,method,job):
                self.assertEqual(method,"GET")
                return {"jobId":job["jobId"],"state":"completed","operation":{"Operation":{"state":"completed"}}}
            with mock.patch.object(service.rollout,"transport",return_value=request):
                for _ in range(4):state=service.advance(path,plan["rolloutId"],{})
            self.assertEqual(state["phase"],"completed")
            db=rollout.connection(path)
            try:self.assertEqual(db.execute("SELECT count(*) FROM events").fetchone()[0],4)
            finally:db.close()


if __name__=="__main__":unittest.main()
