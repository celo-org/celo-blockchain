import numpy as np
PREPREPARE = 'PREPREPARE'
ROUND_CHANGE = 'ROUNDCHANGE'
DOWN = 'DOWN'
UP = 'UP'
TIMEOUT = 'TIMEOUT'

class RoundChangeSet(object):
    def __init__(self):
        self.count_by_round = {}
        self.round_by_validator = {}

    def add(self, index, r):
        prev = self.round_by_validator.get(index, -1)
        if prev < r:
            self.round_by_validator[index] = r
            if prev >= 0:
                self.count_by_round[prev] -= 1
            self.count_by_round[r] = self.count_by_round.get(r, 0) + 1

    def max_on_one_round(self, num):
        rounds = sorted(self.count_by_round.keys(), reverse=True)
        for r in rounds:
            if self.count_by_round[r] >= num:
                return r
        return None

    def max_round(self, num):
        rounds = sorted(self.count_by_round.keys(), reverse=True)
        acc = 0
        for r in rounds:
            acc += self.count_by_round[r]
            if acc >= num:
                return r
        return None

    def clear(self):
        self.count_by_round = {}
        self.round_by_validator = {}


class Validator:
    def __init__(self, index, num_validators, f, round_timer, downtime):
        self.num_validators = num_validators
        self.index = index
        self.round_timer = round_timer
        self.f = f
        self.cur_round = 0
        self.desired_round = 0
        self.last_proposer = 0
        self.last_block_time = 0
        self.is_up = True
        self.round_change_set = RoundChangeSet()
        self.next_timeout = self.last_block_time + self.round_timer(0)
        self.sent_preprepare_for_round = {}
        self.downtime = Downtime(downtime)

    def next_seq(self, last_proposer, last_block_time):
        # Record these values even when down, since they will be used when we come back up.
        self.last_proposer = last_proposer
        self.last_block_time = last_block_time
        if (self.is_up):
            self.next_timeout = last_block_time + self.round_timer(0)
            self.cur_round = 0
            self.desired_round = 0
            self.round_change_set.clear()
            self.sent_preprepare_for_round = {}

    def is_proposer(self):
        return self.index == (self.cur_round + self.last_proposer + 1) % self.num_validators

    def get_next_event(self, t):
        # If we're down, the only event we can fire is to come back up.
        if not self.is_up:
            return (UP, self.cur_round, self.index, self.downtime.next_up(t))
        downtime_event = (DOWN, self.cur_round, self.index, self.downtime.next_down(t))
        timeout_event = (TIMEOUT, self.cur_round, self.index, self.next_timeout)
        possible_events = [downtime_event, timeout_event]
        if self.cur_round == self.desired_round and self.is_proposer() and not self.sent_preprepare_for_round.get(self.cur_round, False):
            next_proposal = max(self.last_block_time + 5, t)
            proposal_event = (PREPREPARE, self.cur_round, self.index, next_proposal)
            possible_events.append(proposal_event)
        return min(possible_events, key=lambda e: e[3])

    def handle_event(self, event):
        #print 'Validator {} handling {} event at time {} round {} desired round {}'.format(self.index, event[0], event[3], self.cur_round, self.desired_round)
        if (self.index != event[2]):
            raise Exception('Handling event not intended for node')
        if event[0] == UP:
            self.come_up(event[3])
            return None
        elif event[0] == DOWN:
            self.go_down()
            return None
        elif event[0] == TIMEOUT:
            return self.ff_round(self.desired_round + 1, event[3])[0]
        elif event[0] == PREPREPARE:
            self.sent_preprepare_for_round[self.cur_round] = True
            return (PREPREPARE, self.cur_round, self.index)
        else:
            raise Exception('Invlid event')

    def go_down(self):
        self.cur_round = 0
        self.desired_round = 0
        self.is_up = False
        self.round_change_set.clear()
        self.next_timeout = None
        self.sent_preprepare_for_round = {}
        self.last_proposer = 0
        self.last_block_time = 0

    def come_up(self, t):
        self.is_up = True
        self.next_timeout = t + self.round_timer(0)

    def handle_msg(self, msg_type, msg_round, index, time):
        if msg_type == ROUND_CHANGE:
            return self.handle_round_change(msg_round, index, time)
        else:
            return self.handle_preprepare(msg_round, time)

    def handle_round_change(self, r, index, t):
        if not self.is_up:
            return []
        self.round_change_set.add(index, r)
        r_quorum = self.round_change_set.max_on_one_round(2 * self.f + 1)
        if r_quorum is not None:
            return self.round_change(r_quorum, t)
        r_ff = self.round_change_set.max_round(2 * self.f + 1)
        if r_ff is not None:
            return self.ff_round(r_ff, t)
        return[]

    def handle_preprepare(self, r, t):
        if not self.is_up:
            return False
        if self.desired_round <= r:
            return True
        return False

    def ff_round(self, r, t):
        if (r > self.desired_round):
            #print 'Validator {} fast forward to round {}'.format(self.index, r)
            self.desired_round = r
            self.next_timeout = t + self.round_timer(r)
            return [(ROUND_CHANGE, r, self.index)]
        return []

    def round_change(self, r, t):
        if (r >= self.desired_round and r > self.cur_round):
            #print 'Validator {} move to round {}'.format(self.index, r)
            self.cur_round = r
            self.desired_round = r
            self.next_timeout = t + self.round_timer(r)
            return [(ROUND_CHANGE, r, self.index)]
        return []

class Downtime(object):
    def __init__(self, ranges):
        self.ranges = ranges

    def next_down(self, t):
        for r in self.ranges:
            if t <= r[0]:
                return r[0]
        return float('inf')

    def next_up(self, t):
        for r in self.ranges:
            if t <= r[1]:
                return r[1]
        return float('inf')

def step(validators, cur_time, f):
    #print 'Step: t = {}'.format(cur_time)
    # First, advance in time to the next possible event.
    next_event = min([v.get_next_event(cur_time) for v in validators], key = lambda e: e[3])
    if next_event == float('inf'):
        raise Exception('All nodes down for next event')
    cur_time = next_event[3]
    msg = validators[next_event[2]].handle_event(next_event)
    # When coming up or down, we don't send any messages.
    if msg is None:
        return (False, cur_time)
    # If it's a PREPREPARE, see how many validators are willing to accept it. If we have a quorum
    # move to the next sequence.
    if msg[0] == PREPREPARE:
        responses = [v.handle_msg(msg[0], msg[1], msg[2], cur_time) for v in validators]
        if sum(responses) >= 2 * f + 1:
            for v in validators:
                v.next_seq(msg[2], cur_time)
            return (True, cur_time)
        else:
            return (False, cur_time)
    else:
        msgs = [msg]
        while(len(msgs) > 0):
            msg = msgs.pop(0)
            msg_type = msg[0]
            msg_round = msg[1]
            msg_index = msg[2]
            for v in validators:
                msgs.extend(v.handle_msg(msg_type, msg_round, msg_index, cur_time))
        return (False, cur_time)

def sim(timeout_fn, num_validators, f, downtimes, length):
    t = 0
    block = 0
    validators = [Validator(i, num_validators, f, timeout_fn, downtimes[i]) for i in range(num_validators)]
    while (t < length):
        block_created, t = step(validators, t, f)
        if block_created:

            #if (block % 10 == 0):
            #    print '---------------CREATED BLOCK {} at time {}-------------------'.format(block, t)
            block += 1
    return block

def exponential_2(r):
    timeout = 3
    if r == 0:
        timeout += 5
    else:
        timeout += 2 ** r
    return timeout

def exponential_110(r):
    timeout = 3
    if r == 0:
        timeout += 5
    else:
        timeout += 1.10 ** r
    return timeout

def mixed(r):
    timeout = 3
    if r == 0:
        timeout += 5
    else:
        timeout += math.max(1.20 ** r, 3*r)
    return timeout


def linear(r):
    timeout = 3
    if r == 0:
        timeout += 5
    else:
        timeout += 3 * r
    return timeout

def get_downtime(num_validators, length, f):
    downtimes = [[] for i in range(num_validators)]
    days = float(length) / (24 * 60 * 60)
    downtime_events = []
    # Add on average, 2 short downtime events per validator per day
    for i in range(num_validators):
        num_events = np.random.poisson(days * 2)
        #print 'Adding {} short downtime events for validator {}'.format(num_events, i)
        for j in range(num_events):
            duration = int(np.random.normal(10 * 60, 60))
            downtime_events.append((duration, [i]))

    # Add on average, 1 medium downtime event per validator every other day
    for i in range(num_validators):
        num_events = np.random.poisson(days / 2)
        #print 'Adding {} medium downtime events for validator {}'.format(num_events, i)
        for j in range(num_events):
            duration = int(np.random.normal(60 * 60, 60 * 60))
            downtime_events.append((duration, [i]))

    # Add on average, 1 medium downtime event affecting many validators per day
    num_events = np.random.poisson(days)
    for i in range(num_events):
        num_validators_affected = int(np.random.normal(f, f/2))
        #print 'Adding a medium downtime event for {} validators'.format(num_validators_affected)
        validators_affected = np.random.choice(num_validators, num_validators_affected, replace=False)
        duration = int(np.random.normal(60 * 60, 60 * 60))
        downtime_events.append((duration, validators_affected))

    for d in downtime_events:
        start_time = int(np.random.random() * length)
        for v in d[1]:
            downtimes[v].append([start_time, start_time + d[0]])

    merged_downtimes = [[] for i in range(num_validators)]
    # Sort by start time and merge overlapping downtime events
    for i in range(num_validators):
        downtimes[i] = sorted(downtimes[i], key = lambda d: d[0])
        while(len(downtimes[i]) > 0):
            downtime = downtimes[i].pop(0)
            if len(merged_downtimes[i]) == 0:
                merged_downtimes[i].append(downtime)
            else:
                merged = merged_downtimes[i][-1]
                if downtime[0] <= merged[1]:
                    #print 'merging {} and {}'.format(merged, downtime)
                    merged_downtimes[i][-1] = [merged[0], downtime[1]]
                else:
                    merged_downtimes[i].append(downtime)
    return merged_downtimes

num_validators = 100
f = 33
length = 7 * 24 * 60 * 60
trials = 10

for i in range(trials):
    np.random.seed(i)
    downtimes = get_downtime(num_validators, length, f)
    #e = sim(exponential_2, num_validators, f, downtimes, length)
    e = sim(exponential_110, num_validators, f, downtimes, length)
    #l = sim(linear, num_validators, f, downtimes, length)
    l = 0
    print 'Trial {}, exponential mined {} blocks, linear mined {} blocks'.format(i, e, l)

#sim(exponential, 4, 1, [[[0, 60 * 60]], [[0, 60 * 60]], [[0, 100 * 60 * 60]], []], 7754)
