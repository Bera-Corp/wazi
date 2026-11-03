"""Read-only loopback UI preview using the real catalog database, without sign-in."""
import argparse
from pathlib import Path

from .contracts import strict_json
from .server import Application, BoundedServer
from .store import Store


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=3690)
    parser.add_argument("--database", default="artifacts/community-preview/catalog.sqlite")
    args = parser.parse_args()
    store = Store(args.database)
    store.seed(strict_json((Path(__file__).parent / "seeds.json").read_bytes()))
    base = Application(store, "http://127.0.0.1:" + str(args.port), "artifacts/community-preview/objects").handler()
    class ReadOnly(base):
        def do_POST(self):
            self.response(403, {"error": "read-only UI preview"})
            self.close_connection = True
    print("Read-only community preview: http://127.0.0.1:" + str(args.port), flush=True)
    BoundedServer(("127.0.0.1", args.port), ReadOnly).serve_forever()


if __name__ == "__main__":
    main()
