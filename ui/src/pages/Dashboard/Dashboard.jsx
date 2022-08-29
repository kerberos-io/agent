import React from 'react';
import {
  Breadcrumb,
  KPI,
  ImageCard,
  Table,
  TableHeader,
  TableBody,
  TableRow,
  Icon,
  Ellipse,
  Card,
  SetupBox,
} from '@kerberos-io/ui';
// import { Link } from 'react-router-dom';
import './Dashboard.scss';
import ReactTooltip from 'react-tooltip';

// eslint-disable-next-line react/prefer-stateless-function
class Dashboard extends React.Component {
  render() {
    return (
      <div>
        <Breadcrumb
          title="Dashboard"
          level1="Overview of your video surveilance"
          level1Link=""
        >
          {/* <Link to="/deployments">
            <Button
              label="Add Kerberos Agent"
              icon="plus-circle"
              type="default"
            />
    </Link> */}
        </Breadcrumb>

        <div className="stats grid-container --four-columns">
          <KPI number="69" divider="0" footer="Number of days" />
          <KPI number="1540" divider="0" footer="Total recordings" />
          <Card
            title="Camera"
            subtitle="succesfully connected"
            footer="IP Camera"
            icon="circle-check-big"
          />
          <Card
            title="Cloud"
            subtitle="Not connected"
            footer="Kerberos Hub"
            icon="circle-cross-big"
          />
        </div>
        <hr />
        <div className="stats grid-container --two-columns">
          <div>
            <h2>Latest events</h2>
            <Table>
              <TableHeader>
                <TableRow
                  id="header"
                  headercells={['time', 'description', 'name']}
                />
              </TableHeader>
              <TableBody>
                <TableRow
                  id="cells1"
                  bodycells={[
                    <>
                      <Ellipse status="success" />{' '}
                      <p data-tip="10m and 5s ago">19:45:10</p>
                    </>,
                    <>
                      <p>Motion was detected</p>
                    </>,
                    <>
                      <span className="version">Frontdoor</span>&nbsp;
                      <Icon label="cameras" />
                    </>,
                  ]}
                />
                <TableRow
                  id="cells1"
                  bodycells={[
                    <>
                      <Ellipse status="success" />{' '}
                      <p data-tip="10m and 5s ago">18:23:44</p>
                    </>,
                    <>
                      <p>Motion was detected</p>
                    </>,
                    <>
                      <span>Frontdoor</span>&nbsp;
                      <Icon label="cameras" />
                    </>,
                  ]}
                />
                <TableRow
                  id="cells1"
                  bodycells={[
                    <>
                      <Ellipse status="success" />{' '}
                      <p data-tip="10m and 5s ago">18:20:29</p>
                    </>,
                    <>
                      <p>Motion was detected</p>
                    </>,
                    <>
                      <span className="version">Frontdoor</span>&nbsp;
                      <Icon label="cameras" />
                    </>,
                  ]}
                />
                <TableRow
                  id="cells1"
                  bodycells={[
                    <>
                      <Ellipse status="success" />{' '}
                      <p data-tip="10m and 5s ago">15:16:58</p>
                    </>,
                    <>
                      <p>Motion was detected</p>
                    </>,
                    <>
                      <span className="version">Frontdoor</span>&nbsp;
                      <Icon label="cameras" />
                    </>,
                  ]}
                />
                <TableRow
                  id="cells1"
                  bodycells={[
                    <>
                      <Ellipse status="success" />{' '}
                      <p data-tip="10m and 5s ago">10:05:44</p>
                    </>,
                    <>
                      <p>Motion was detected</p>
                    </>,
                    <>
                      <span className="version">Frontdoor</span>&nbsp;
                      <Icon label="cameras" />
                    </>,
                  ]}
                />
              </TableBody>
            </Table>
          </div>
          <div>
            <h2>Live view</h2>
            <SetupBox
              btnicon="cameras"
              btnlabel="Configure connection"
              dashed
              header="Loading live view"
              text="Hold on we are loading your live view here. If you didn't configure your camera connection, update it on the settings pages."
            />
            <ImageCard imageSrc="http://localhost:8080/stream?token=xxxx" />
          </div>
        </div>
        <ReactTooltip />
      </div>
    );
  }
}
export default Dashboard;
