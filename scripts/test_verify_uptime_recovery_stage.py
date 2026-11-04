import hashlib
import io
import json
from pathlib import Path
import sqlite3
import tarfile
import tempfile
import unittest

from verify_uptime_recovery_stage import verify


class RecoveryStageTests(unittest.TestCase):
    def prepare(self, root, extra=None, corrupt=False):
        stage=root/'stage';stage.mkdir()
        db=root/'database'
        connection=sqlite3.connect(db)
        try:
            for table in ('monitor','setting','user'):connection.execute('CREATE TABLE '+table+' (id INTEGER)')
            connection.commit()
        finally:connection.close()
        with tarfile.open(stage/'payload','w') as archive:
            data=b'invalid database' if corrupt else db.read_bytes()
            item=tarfile.TarInfo('./kuma.db');item.size=len(data);archive.addfile(item,io.BytesIO(data))
            if extra is not None:archive.addfile(extra,io.BytesIO(b''))
        record={'state':'decrypted-staging-only','project':'wazi-uptime-kuma-01','payloadFormat':'volume-tar','plaintextDigest':'sha256:'+hashlib.sha256((stage/'payload').read_bytes()).hexdigest(),'ciphertextDigest':'sha256:'+'a'*64,'manifestDigest':'sha256:'+'b'*64}
        (stage/'recovery-stage.json').write_text(json.dumps(record),encoding='utf-8')
        return stage

    def test_sqlite_integrity_without_application_restore(self):
        with tempfile.TemporaryDirectory() as temporary:
            root=Path(temporary);result=verify(self.prepare(root),root/'verified')
            self.assertEqual(result['sqliteIntegrity'],'ok')
            self.assertFalse(result['applicationRestorePerformed'])

    def test_unsafe_entries_rejected_before_database_write(self):
        for name,kind in (('../escape',tarfile.REGTYPE),('/absolute',tarfile.REGTYPE),('link',tarfile.SYMTYPE),('C:/escape',tarfile.REGTYPE)):
            with self.subTest(name=name),tempfile.TemporaryDirectory() as temporary:
                root=Path(temporary);item=tarfile.TarInfo(name);item.type=kind;item.linkname='kuma.db'
                with self.assertRaises(ValueError):verify(self.prepare(root,item),root/'verified')
                self.assertFalse((root/'verified/kuma.db').exists())
                self.assertFalse((root/'escape').exists())

    def test_corrupt_database_never_receives_success_record(self):
        with tempfile.TemporaryDirectory() as temporary:
            root=Path(temporary)
            with self.assertRaises(sqlite3.DatabaseError):verify(self.prepare(root,corrupt=True),root/'verified')
            self.assertFalse((root/'verified/verification.json').exists())


if __name__=='__main__':unittest.main()
