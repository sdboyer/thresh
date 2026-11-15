"""A choke point in Python: every use of jax.profiler goes through Profiler."""
import jax


class Profiler:
    def start(self, where):
        jax.profiler.start_trace(where)

    def stop(self):
        self.stopped = True
        jax.profiler.stop_trace()


def step(state):
    return state + 1
