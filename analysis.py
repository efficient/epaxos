# -*- coding: utf-8 -*-
import concurrent.futures
from os import listdir, popen
from os.path import isfile, join
from sys import argv


# TODO (highlight): usage:
#   python3.8 analysis_paxos.py print-title to print out parameter & metrics of the report
#   python3.8 analysis_paxos.py <log-folder-path> to print out excel-friendly report

# TODO (highlight): assumptions:
#   1. all client operations are successfully finished
#   2. all client logs are gathered into one directory, and its relative/absolute path is offered by the first argument
#   3. throughput is calculated as the total client operations / the MAXIMUM of client runtime durations
#   4. py2TimeTool.py is located in the same directory

# TODO (highlight): dependencies:
#   Python3.8 and above to support f-string and ordered dictionaries AND
#   Python2 to support time duration conversion (py2TimeTool.py)


# extract experiment parameters from the log folder
# for example, files started with S5-C5-q5000-w100-c50--* are considered as one experiment
# while S7-C5-q5000-w100-c50--* are considered as another experiment
def get_experiments(log_folder_path):
    files = [f for f in listdir(log_folder_path) if isfile(join(log_folder_path, f)) and "out" in f]
    params = set()
    for file in files:
        params.add(file[:file.find("--")])
    # print(params)
    return sorted(params)


# extract the number number of client operation and the runtime from a client's output file
def get_epaxos_client_statistics(log_file_path):
    """
    each file looks like:
    Zipfian distribution:
    Round took 2.66067856s
    Test took 2.660755695s
    Successful: 500000
    """
    duration, operations = 0, 0
    with open(log_file_path) as f:
        lines = f.readlines()
    for line in lines:
        if "Test took" in line:
            line = line[len("Test took"):]
            duration = round(float(popen(f"python2.7 py2TimeTool.py {line}").read().strip()), 2)
        if "Successful:" in line:
            line = line[len("Successful:"):].strip()
            operations = int(line)
    return duration, operations


def get_epaxos_client_statistics_v2(log_file_path):
    """
    each file looks like:
    Zipfian distribution:
    Round took 2.66067856ms
    Round took 1.66067856ms
    Round took 0.66067856ms
    ...
    Test took 4.98203568ms
    Successful: 500000
    """
    latencies = []  # in ms
    result = {}
    with open(log_file_path) as f:
        lines = f.readlines()
    for line in lines:
        if "Round took" in line:
            line = line[len("Round took"):]
            if "ms" in line:
                latencies.append(float(line.strip()[:-2]))
            elif "µs" in line:
                latencies.append(float(line.strip()[:-2]) / 1000)
            else:
                print(line, line, line)
        # if "Successful:" in line:
        #     line = line[len("Successful:"):].strip()
        #     result["operations"] = int(line)
    latencies = latencies[int(len(latencies) * 0.1): int(len(latencies) * 0.9)]
    latencies.sort()
    # print(latencies)

    result["duration"] = sum(latencies)
    result["operations"] = len(latencies)
    result["average"] = sum(latencies) / len(latencies)
    result["median"] = latencies[int(len(latencies) * 0.5)]
    result["95%tile"] = latencies[int(len(latencies) * 0.95)]
    result["99%tile"] = latencies[int(len(latencies) * 0.99)]
    return result


def analysis_epaxos_logs():
    params = get_experiments(argv[1])
    infos = []
    for param in params:
        print(param)
        sp = param.split("-")
        info = {
            "S": (int(sp[0][1:]), "num of servers"),
            "C": (int(sp[1][1:]), "num of clients"),
            "q": (int(sp[2][1:]), "num of requests per client"),
            "w": (int(sp[3][1:]), "percentile of writes"),
            "r": (int(sp[4][1:]), "num of rounds"),
            "b": (int(sp[2][1:]) // int(sp[4][1:]), "client batch size"),
            "c": (int(sp[5][1:]), "write conflict"),
            "clientDurations": [],
        }
        if info["r"][0] == 1:  # open-loop
            for i in range(info["C"][0]):
                file = join(argv[1], f"{param}--client{i}.out")
                dur, ops = get_epaxos_client_statistics(file)
                assert ops == info["q"][0], f"client {i}'s successful operations != num of requests per client"
                info["clientDurations"].append(dur)
            assert len(info["clientDurations"]) == info["C"][0], "actual number of clients != parameter"
            info["totalRequests"] = (info["C"][0] * info["q"][0], "the total number of client requests")
            info["clientAverageDuration"] = (
                round(sum(info["clientDurations"]) / info["C"][0], 2), "client average runtime (sec)")
            info["clientMaximumDuration"] = (max(info["clientDurations"]), "client maximum runtime (sec)")
            info["throughput"] = (round(info["totalRequests"][0] / info["clientMaximumDuration"][0], 2),
                                  "total client requests / max. runtime (ops/sec)")
            infos.append(info)
        else:  # closed loop
            with concurrent.futures.ThreadPoolExecutor() as executor:
                futures = []
                for i in range(info["C"][0]):
                    file = join(argv[1], f"{param}--client{i}.out")
                    futures.append(executor.submit(get_epaxos_client_statistics_v2, file))
                futures_res = [f.result() for f in futures]

            info["totalRequests"] = (info["C"][0] * info["q"][0], "the total number of client requests")

            client_avg_duration = sum([d["duration"] for d in futures_res]) / len(futures_res) / 1000  # ms - sec
            info["clientAvgDuration"] = (round(client_avg_duration, 2), "client Avg Duration -- mid 80% (sec)")

            client_avg_latency = sum([d["average"] for d in futures_res]) / len(futures_res)
            info["clientAvgLatency"] = (round(client_avg_latency, 2), "client Avg Latency -- mid 80% (ms)")

            client_p50_latency = sum([d["median"] for d in futures_res]) / len(futures_res)
            info["clientp50Latency"] = (round(client_p50_latency, 2), "client Median Latency -- mid 80% (ms)")

            client_p95_latency = sum([d["95%tile"] for d in futures_res]) / len(futures_res)
            info["clientp95Latency"] = (round(client_p95_latency, 2), "client 95%tile Latency -- mid 80% (ms)")

            client_p99_latency = sum([d["99%tile"] for d in futures_res]) / len(futures_res)
            info["clientp99Latency"] = (round(client_p99_latency, 2), "client 99%tile Latency -- mid 80% (ms)")

            info["clientMaximumDuration"] = (max([d["duration"] for d in futures_res]) / 1000,
                                             "client maximum runtime (sec)")

            clientthroughputs = [d['operations'] * info["b"][0] / (d['duration'] / 1000) for d in futures_res]
            info["throughput"] = (
                round(sum(clientthroughputs), 2), "sum of (client mid 80% requests * batch size / mid 80% time) (ops/sec)")
            infos.append(info)
        return infos  # assume there's one param only


if __name__ == '__main__':
    infos = analysis_epaxos_logs()
    if "print-title" in argv:
        if len(infos) != 0:
            for k, v in infos[0].items():
                if len(v) > 1 and not isinstance(v, list):
                    print(v[1], end=", ")
            print()
    for info in infos:
        for k, v in info.items():
            if len(v) > 1 and not isinstance(v, list):
                print(v[0], end=", ")
        print()