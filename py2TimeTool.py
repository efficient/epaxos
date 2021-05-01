# -*- coding:utf-8 -*-
# adapted from https://www.itread01.com/content/1527482333.html

import logging
import sys

reload(sys)  # reload 才能調用 setdefaultencoding 方法
sys.setdefaultencoding('utf-8')  # 設置 'utf-8'


# from cfg import config

class TimeTool(object):
    def __init__(self):
        handler = logging.StreamHandler()
        formatter = logging.Formatter('%(asctime)s %(name)-12s %(levelname)-8s %(message)s')
        handler.setFormatter(formatter)
        self.logger = logging.getLogger("HistoryReader")
        self.logger.addHandler(handler)
        # self.logger.setLevel(config.LOG_LEVEL)
        self.Nanosecond = 1
        self.Microsecond = 1000 * self.Nanosecond
        self.Millisecond = 1000 * self.Microsecond
        self.Second = 1000 * self.Millisecond
        self.Minute = 60 * self.Second
        self.Hour = 60 * self.Minute
        self.unitMap = {
            "ns": int(self.Nanosecond),
            "us": int(self.Microsecond),
            "μs": int(self.Microsecond),  # U+00B5 = micro symbol
            "μs": int(self.Microsecond),  # U+03BC = Greek letter mu
            "ms": int(self.Millisecond),
            "s": int(self.Second),
            "m": int(self.Minute),
            "h": int(self.Hour),
        }
        pass

    def leadingInt(self, s):
        x, rem, err = int(0), str(""), "time: bad [0-9]*"
        i = 0
        while i < len(s):
            c = s[i]
            if c < '0' or c > '9':
                break
            # print x
            if x > (1 << 63 - 1) / 10:
                # print "x > (1 << 63-1)/10 => %s > %s" %(x, (1 << 63-1)/10)
                return 0, "", err
            x = x * 10 + int(c) - int('0')
            if x < 0:
                # print "x < 0 => %s < 0" %(x)
                return 0, "", err
            i += 1
        return x, s[i:], None

    def leadingFraction(self, s):
        x, scale, rem = int(0), float(1), ""
        i, overflow = 0, False
        while i < len(s):
            c = s[i]
            if c < '0' or c > '9':
                break
            if overflow:
                continue
            if x > (1 << 63 - 1) / 10:
                overflow = True
                continue
            y = x * 10 + int(c) - int('0')
            if y < 0:
                overflow = True
                continue
            x = y
            scale *= 10
            i += 1
        return x, scale, s[i:]

    """
    將小時，分鐘，轉換為秒
    比如： 5m 轉換為 300秒；5m20s 轉換為320秒
    time 單位支持："ns", "us" (or "μs"), "ms", "s", "m", "h"
    """

    def ParseDuration(self, s):
        if s == "" or len(s) < 1:
            return 0

        orig = s
        neg = False
        d = float(0)

        if s != "":
            if s[0] == "-" or s[0] == "+":
                neg = s[0] == "-"
                s = s[1:]

        if s == "0" or s == "":
            return 0

        while s != "":
            v, f, scale = int(0), int(0), float(1)

            # print "S: %s" %s
            # the next character must be [0-9.]
            if not (s[0] == "." or '0' <= s[0] and s[0] <= '9'):
                self.logger.error("time1: invalid duration %s, s:%s" % (orig, s))
                return 0

            # Consume [0-9]*
            pl = len(s)
            v, s, err = self.leadingInt(s)
            if err != None:
                self.logger.error("time2, invalid duration %s" % orig)
                return 0
            pre = pl != len(s)

            # consume (\.[0-9]*)?
            post = False
            if s != "" and s[0] == ".":
                s = s[1:]
                pl = len(s)
                f, scale, s = self.leadingFraction(s)
                post = pl != len(s)
            if not pre and not post:
                self.logger.error("time3, invalid duration %s" % orig)
                return 0

            # Consume unit.
            i = 0
            while i < len(s):
                c = s[i]
                if c == '.' or '0' <= c and c <= '9':
                    break
                i += 1
            if i == 0:
                self.logger.error("time4: unkonw unit in duration: %s" % orig)
                return 0
            # print "s:%s, i:%s, s[:i]:%s" %(s, i, s[:i])
            u = s[:i]
            s = s[i:]
            if not self.unitMap.has_key(u):
                self.logger.error("time5: unknow unit %s in duration %s" % (u, orig))
                return 0
            unit = self.unitMap[u]
            if v > (1 << 63 - 1) / unit:
                self.logger.error("time6: invalid duration %s" % orig)
                return 0
            v *= unit
            if f > 0:
                v += int(float(f) * (float(unit) / scale))
                if v < 0:
                    self.logger.error("time7: invalid duration %s" % orig)
                    return 0
            d += v
            if d < 0:
                self.logger.error("time8: invalid duration %s" % orig)
                return 0

        if neg:
            d = -d
        return float(d)


if __name__ == "__main__":
    from sys import argv

    tools = TimeTool()
    # s = tools.ParseDuration("1m20.123s")
    s = tools.ParseDuration(argv[1])
    print(s / tools.Second)